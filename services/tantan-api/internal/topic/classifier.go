package topic

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type GeneratedSet struct {
	Topics []GeneratedTopic
}

type GeneratedTopic struct {
	Name     string
	EntryIDs []string
}

type classificationDocument struct {
	ID      string
	Title   string
	Body    string
	Source  string
	Weights map[string]float64
	Labels  map[string]string
}

type termCandidate struct {
	Key      string
	Label    string
	Score    float64
	EntryIDs []string
}

var classifierStopWords = map[string]struct{}{
	"a": {}, "an": {}, "and": {}, "are": {}, "as": {}, "at": {}, "be": {}, "by": {}, "for": {},
	"from": {}, "how": {}, "in": {}, "is": {}, "it": {}, "new": {}, "of": {}, "on": {}, "or": {},
	"that": {}, "the": {}, "this": {}, "to": {}, "using": {}, "with": {}, "you": {}, "your": {},
	"一个": {}, "一种": {}, "以及": {}, "使用": {}, "关于": {}, "发布": {}, "如何": {}, "我们": {},
	"什么": {}, "这个": {}, "这些": {}, "最新": {}, "内容": {}, "文章": {}, "消息": {}, "新闻": {},
	"br": {}, "href": {}, "http": {}, "https": {}, "img": {}, "nofollow": {}, "noopener": {},
	"noreferrer": {}, "rel": {}, "rt": {}, "src": {}, "target": {}, "blank": {}, "true": {},
	"untitled": {}, "www": {}, "com": {},
}

type classifierLexicon struct {
	Name     string
	Keywords []string
}

// The lexicon is deliberately local, deterministic and explainable. Categories
// appear only when the current unread corpus contains matching documents.
var classifierLexicons = []classifierLexicon{
	{Name: "AI", Keywords: []string{"artificial intelligence", "machine learning", "openai", "anthropic", "claude", "gemini", "grok", "llm", "language model", "hugging face", "prompt", "ai"}},
	{Name: "Agent", Keywords: []string{"multiagent", "multi-agent", "agentic", "agent", "tool calling", "claude code", "codex", "automode", "auto mode", "mcp"}},
	{Name: "编程", Keywords: []string{"github", "sqlite", "database", "coding", "code", "developer", "javascript", "typescript", "python", "react", "css", "api", "software", "programming"}},
	{Name: "图像视频", Keywords: []string{"image editing", "image model", "generate images", "video", "multimodal", "multi-modal", "rendering", "3d", "meme", "imagine"}},
	{Name: "航天", Keywords: []string{"spacex", "starship", "falcon 9", "starlink", "satellite", "orbit", "rocket", "spaceflight"}},
	{Name: "时事", Keywords: []string{"election", "senator", "congress", "politic", "socialist", "government", "violence", "detention", "america act", "wokeness"}},
	{Name: "Web3", Keywords: []string{"web3", "blockchain", "bitcoin", "ethereum", "crypto", "defi", "wallet", "token"}},
	{Name: "产品", Keywords: []string{"productivity", "startup", "launching", "revenue", "subscription", "tesla", "factory", "design"}},
	{Name: "科学", Keywords: []string{"research", "science", "physics", "biology", "astronomy", "paper", "experiment"}},
}

func (service *Service) RefreshGenerated(ctx context.Context, userID string) error {
	set, err := service.Classify(ctx, userID, nil)
	if err != nil {
		return err
	}
	matches, err := service.generatedSetMatches(ctx, userID, "dynamic", set)
	if err != nil {
		return err
	}
	if matches {
		return nil
	}
	return service.ReplaceGenerated(ctx, userID, "dynamic", set)
}

func (service *Service) generatedSetMatches(ctx context.Context, userID, kind string, set GeneratedSet) (bool, error) {
	var count int
	if err := service.store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM topics WHERE user_id=?", userID).Scan(&count); err != nil {
		return false, fmt.Errorf("count generated topics: %w", err)
	}
	if count != len(set.Topics) {
		return false, nil
	}
	for _, expected := range set.Topics {
		topicID := generatedID(userID, kind, NormalizeName(displayName(expected.Name)))
		var storedName, storedKind string
		if err := service.store.DB().QueryRowContext(ctx, "SELECT name,kind FROM topics WHERE user_id=? AND topic_id=?", userID, topicID).Scan(&storedName, &storedKind); errors.Is(err, sql.ErrNoRows) {
			return false, nil
		} else if err != nil {
			return false, fmt.Errorf("read generated topic: %w", err)
		}
		if storedKind != kind || NormalizeName(storedName) != NormalizeName(expected.Name) {
			return false, nil
		}
		rows, err := service.store.DB().QueryContext(ctx, "SELECT entry_id FROM entry_topics WHERE user_id=? AND topic_id=? ORDER BY entry_id", userID, topicID)
		if err != nil {
			return false, fmt.Errorf("read generated assignments: %w", err)
		}
		storedEntries := make([]string, 0, len(expected.EntryIDs))
		for rows.Next() {
			var entryID string
			if err := rows.Scan(&entryID); err != nil {
				rows.Close()
				return false, fmt.Errorf("scan generated assignment: %w", err)
			}
			storedEntries = append(storedEntries, entryID)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return false, fmt.Errorf("iterate generated assignments: %w", err)
		}
		if err := rows.Close(); err != nil {
			return false, fmt.Errorf("close generated assignments: %w", err)
		}
		expectedEntries := append([]string(nil), expected.EntryIDs...)
		sort.Strings(expectedEntries)
		if len(storedEntries) != len(expectedEntries) {
			return false, nil
		}
		for index := range expectedEntries {
			if storedEntries[index] != expectedEntries[index] {
				return false, nil
			}
		}
	}
	return true, nil
}

func (service *Service) Classify(ctx context.Context, userID string, entryIDs []string) (GeneratedSet, error) {
	documents, err := service.loadClassificationDocuments(ctx, userID, entryIDs)
	if err != nil {
		return GeneratedSet{}, err
	}
	return classifyDocuments(documents), nil
}

func (service *Service) loadClassificationDocuments(ctx context.Context, userID string, entryIDs []string) ([]classificationDocument, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, errors.New("classification user is required")
	}
	query := `
SELECT e.entry_id,e.title,substr(trim(COALESCE(e.description,'') || ' ' || COALESCE(e.content,'')),1,2000),f.title
FROM account_entries ae
JOIN entries e ON e.entry_id=ae.entry_id
JOIN feeds f ON f.feed_id=e.feed_id
WHERE ae.user_id=? AND ae.read_at IS NULL`
	arguments := []any{userID}
	if len(entryIDs) == 0 {
		now := service.now().UTC()
		query += " AND julianday(e.published_at)>=julianday(?) AND julianday(e.published_at)<=julianday(?)"
		arguments = append(arguments, now.Add(-7*24*time.Hour).Format(time.RFC3339Nano), now.Add(time.Second).Format(time.RFC3339Nano))
	} else {
		if len(entryIDs) > 100 {
			return nil, errors.New("classification entry limit exceeded")
		}
		query += " AND e.entry_id IN (" + strings.TrimRight(strings.Repeat("?,", len(entryIDs)), ",") + ")"
		for _, entryID := range entryIDs {
			arguments = append(arguments, entryID)
		}
	}
	query += " ORDER BY e.published_at DESC,e.entry_id LIMIT 100"
	rows, err := service.store.DB().QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, fmt.Errorf("load classification documents: %w", err)
	}
	defer rows.Close()
	documents := make([]classificationDocument, 0, 100)
	for rows.Next() {
		var document classificationDocument
		if err := rows.Scan(&document.ID, &document.Title, &document.Body, &document.Source); err != nil {
			return nil, fmt.Errorf("scan classification document: %w", err)
		}
		document.Weights = make(map[string]float64)
		document.Labels = make(map[string]string)
		addTerms(document.Weights, document.Labels, document.Title, 3)
		addTerms(document.Weights, document.Labels, document.Body, 1)
		addTerms(document.Weights, document.Labels, document.Source, 0.5)
		documents = append(documents, document)
	}
	return documents, rows.Err()
}

func classifyDocuments(documents []classificationDocument) GeneratedSet {
	if len(documents) == 0 {
		return GeneratedSet{Topics: []GeneratedTopic{}}
	}
	documentFrequency := make(map[string]int)
	labels := make(map[string]string)
	for _, document := range documents {
		for key := range document.Weights {
			documentFrequency[key]++
			if labels[key] == "" {
				labels[key] = document.Labels[key]
			}
		}
	}
	minimumFrequency := 1
	if len(documents) >= 2 {
		minimumFrequency = 2
	}
	candidates := make([]termCandidate, 0, len(documentFrequency))
	for key, frequency := range documentFrequency {
		if frequency < minimumFrequency || frequency == len(documents) || !usefulTopicLabel(labels[key]) {
			continue
		}
		entryIDs := make([]string, 0, frequency)
		termFrequency := 0.0
		for _, document := range documents {
			if weight := document.Weights[key]; weight > 0 {
				entryIDs = append(entryIDs, document.ID)
				termFrequency += weight
			}
		}
		idf := math.Log(1 + float64(len(documents))/float64(frequency))
		phraseBoost := 1.0
		if strings.Contains(key, " ") {
			phraseBoost = 1.3
		}
		candidates = append(candidates, termCandidate{Key: key, Label: labels[key], EntryIDs: entryIDs, Score: termFrequency * idf * phraseBoost})
	}
	sort.SliceStable(candidates, func(left, right int) bool {
		if candidates[left].Score != candidates[right].Score {
			return candidates[left].Score > candidates[right].Score
		}
		if len(candidates[left].EntryIDs) != len(candidates[right].EntryIDs) {
			return len(candidates[left].EntryIDs) > len(candidates[right].EntryIDs)
		}
		return candidates[left].Key < candidates[right].Key
	})
	target := int(math.Round(math.Sqrt(float64(len(documents)))))
	if target < 3 {
		target = 3
	}
	if target > 7 {
		target = 7
	}
	selected := make([]termCandidate, 0, target)
	for _, candidate := range lexiconCandidates(documents, minimumFrequency) {
		if len(selected) >= target {
			break
		}
		if !redundantCandidate(candidate, selected) {
			selected = append(selected, candidate)
		}
	}
	for _, candidate := range candidates {
		if len(selected) >= target {
			break
		}
		if redundantCandidate(candidate, selected) {
			continue
		}
		selected = append(selected, candidate)
	}
	if len(selected) == 0 {
		fallback := documents[0]
		name := bestFallbackLabel(fallback)
		if name != "" {
			selected = append(selected, termCandidate{Label: name, EntryIDs: []string{fallback.ID}})
		}
	}
	result := GeneratedSet{Topics: make([]GeneratedTopic, 0, len(selected))}
	for _, candidate := range selected {
		result.Topics = append(result.Topics, GeneratedTopic{Name: candidate.Label, EntryIDs: append([]string(nil), candidate.EntryIDs...)})
	}
	return result
}

func addTerms(weights map[string]float64, labels map[string]string, value string, boost float64) {
	words := splitWords(stripClassifierMarkup(value))
	for _, word := range words {
		key := strings.ToLower(word)
		if !usefulTopicLabel(word) {
			continue
		}
		weights[key] += boost
		if labels[key] == "" {
			labels[key] = word
		}
	}
	for index := 0; index+1 < len(words); index++ {
		left, right := words[index], words[index+1]
		if !usefulTopicLabel(left) || !usefulTopicLabel(right) {
			continue
		}
		label := left + " " + right
		if utf8.RuneCountInString(label) > 20 {
			continue
		}
		key := strings.ToLower(label)
		weights[key] += boost * 1.2
		if labels[key] == "" {
			labels[key] = label
		}
	}
}

func splitWords(value string) []string {
	var words []string
	var current []rune
	flush := func() {
		if len(current) == 0 {
			return
		}
		word := strings.TrimSpace(string(current))
		if utf8.RuneCountInString(word) >= 2 && utf8.RuneCountInString(word) <= 20 {
			words = append(words, word)
		}
		current = current[:0]
	}
	for _, value := range []rune(value) {
		if unicode.IsLetter(value) || unicode.IsDigit(value) || value == '+' || value == '#' || value == '.' || value == '-' {
			current = append(current, value)
			continue
		}
		flush()
	}
	flush()
	return words
}

func usefulTopicLabel(value string) bool {
	value = strings.TrimSpace(value)
	count := utf8.RuneCountInString(value)
	if count < 2 || count > 20 {
		return false
	}
	if _, stopped := classifierStopWords[strings.ToLower(value)]; stopped {
		return false
	}
	if count <= 2 && isASCIIWord(value) && !strings.EqualFold(value, "AI") && !strings.EqualFold(value, "3D") {
		return false
	}
	hasLetter := false
	for _, value := range []rune(value) {
		if unicode.IsLetter(value) {
			hasLetter = true
			break
		}
	}
	return hasLetter
}

func lexiconCandidates(documents []classificationDocument, minimumFrequency int) []termCandidate {
	candidates := make([]termCandidate, 0, len(classifierLexicons))
	for _, lexicon := range classifierLexicons {
		entryIDs := make([]string, 0, len(documents))
		score := 0.0
		for _, document := range documents {
			documentScore := classifierLexiconScore(document, lexicon.Keywords)
			if documentScore <= 0 {
				continue
			}
			entryIDs = append(entryIDs, document.ID)
			score += documentScore
		}
		if len(entryIDs) < minimumFrequency {
			continue
		}
		candidates = append(candidates, termCandidate{
			Key:      "semantic:" + strings.ToLower(lexicon.Name),
			Label:    lexicon.Name,
			Score:    score,
			EntryIDs: entryIDs,
		})
	}
	sort.SliceStable(candidates, func(left, right int) bool {
		if candidates[left].Score != candidates[right].Score {
			return candidates[left].Score > candidates[right].Score
		}
		if len(candidates[left].EntryIDs) != len(candidates[right].EntryIDs) {
			return len(candidates[left].EntryIDs) > len(candidates[right].EntryIDs)
		}
		return candidates[left].Key < candidates[right].Key
	})
	return candidates
}

func classifierLexiconScore(document classificationDocument, keywords []string) float64 {
	score := 0.0
	for _, keyword := range keywords {
		if classifierContains(document.Title, keyword) {
			score += 4
		}
		if classifierContains(document.Body, keyword) {
			score += 1
		}
		if classifierContains(document.Source, keyword) {
			score += 0.5
		}
	}
	return score
}

func classifierContains(value, keyword string) bool {
	value = " " + strings.ToLower(stripClassifierMarkup(value)) + " "
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	if keyword == "" {
		return false
	}
	if strings.IndexFunc(keyword, func(value rune) bool { return value > unicode.MaxASCII }) >= 0 {
		return strings.Contains(value, keyword)
	}
	return strings.Contains(value, " "+keyword+" ")
}

func stripClassifierMarkup(value string) string {
	var output strings.Builder
	output.Grow(len(value))
	inTag := false
	for _, current := range value {
		switch {
		case current == '<':
			inTag = true
			output.WriteByte(' ')
		case current == '>':
			inTag = false
			output.WriteByte(' ')
		case inTag:
			continue
		case unicode.IsLetter(current) || unicode.IsDigit(current) || current == '+' || current == '#' || current == '.' || current == '-':
			output.WriteRune(current)
		default:
			output.WriteByte(' ')
		}
	}
	words := strings.Fields(output.String())
	filtered := words[:0]
	for _, word := range words {
		lower := strings.ToLower(word)
		if strings.HasPrefix(lower, "http") || strings.HasPrefix(lower, "www.") || strings.Contains(lower, ".com/") {
			continue
		}
		filtered = append(filtered, word)
	}
	return strings.Join(filtered, " ")
}

func isASCIIWord(value string) bool {
	for _, current := range value {
		if current > unicode.MaxASCII || (!unicode.IsLetter(current) && !unicode.IsDigit(current)) {
			return false
		}
	}
	return true
}

func redundantCandidate(candidate termCandidate, selected []termCandidate) bool {
	for _, existing := range selected {
		if strings.Contains(candidate.Key, existing.Key) || strings.Contains(existing.Key, candidate.Key) {
			return true
		}
		intersection := 0
		set := make(map[string]struct{}, len(existing.EntryIDs))
		for _, entryID := range existing.EntryIDs {
			set[entryID] = struct{}{}
		}
		for _, entryID := range candidate.EntryIDs {
			if _, ok := set[entryID]; ok {
				intersection++
			}
		}
		union := len(existing.EntryIDs) + len(candidate.EntryIDs) - intersection
		if union > 0 && float64(intersection)/float64(union) >= 0.8 {
			return true
		}
	}
	return false
}

func bestFallbackLabel(document classificationDocument) string {
	var best string
	bestWeight := 0.0
	for key, weight := range document.Weights {
		if weight > bestWeight && usefulTopicLabel(document.Labels[key]) {
			best = document.Labels[key]
			bestWeight = weight
		}
	}
	return best
}

func (service *Service) ReplaceGenerated(ctx context.Context, userID, kind string, set GeneratedSet) error {
	return service.store.Write(ctx, func(transaction *sql.Tx) error {
		return service.ReplaceGeneratedTx(ctx, transaction, userID, kind, set)
	})
}

func (service *Service) ReplaceGeneratedTx(ctx context.Context, transaction *sql.Tx, userID, kind string, set GeneratedSet) error {
	if transaction == nil || strings.TrimSpace(userID) == "" || (kind != "dynamic" && kind != "filter") || len(set.Topics) > 7 {
		return errors.New("valid generated topic set is required")
	}
	oldEntries, err := topicEntryIDs(ctx, transaction, userID)
	if err != nil {
		return err
	}
	if _, err := transaction.ExecContext(ctx, "DELETE FROM entry_topics WHERE user_id=?", userID); err != nil {
		return fmt.Errorf("clear generated topic assignments: %w", err)
	}
	if _, err := transaction.ExecContext(ctx, "DELETE FROM topics WHERE user_id=?", userID); err != nil {
		return fmt.Errorf("clear generated topics: %w", err)
	}
	now := service.now().UTC()
	timestamp := now.Format(time.RFC3339Nano)
	seenNames := make(map[string]struct{}, len(set.Topics))
	primary := make(map[string]struct{})
	newEntries := make([]string, 0, 100)
	for index, group := range set.Topics {
		name := displayName(group.Name)
		normalized := NormalizeName(name)
		if normalized == "" || len(group.EntryIDs) == 0 {
			return errors.New("generated topic set contains an empty topic")
		}
		if _, duplicate := seenNames[normalized]; duplicate {
			return errors.New("generated topic set contains duplicate names")
		}
		seenNames[normalized] = struct{}{}
		var stableUntil any
		if kind == "dynamic" {
			stableUntil = now.Add(7 * 24 * time.Hour).Format(time.RFC3339Nano)
		}
		topicID := generatedID(userID, kind, normalized)
		if _, err := transaction.ExecContext(ctx, `
INSERT INTO topics(topic_id,user_id,name,normalized_name,kind,pinned,hidden,sort_order,stable_until,created_at,updated_at)
VALUES(?,?,?,?,?,0,0,?,?,?,?)`, topicID, userID, name, normalized, kind, (index+1)*10, stableUntil, timestamp, timestamp); err != nil {
			return fmt.Errorf("insert generated topic: %w", err)
		}
		seenEntries := make(map[string]struct{}, len(group.EntryIDs))
		for _, entryID := range group.EntryIDs {
			if _, duplicate := seenEntries[entryID]; duplicate {
				return errors.New("generated topic contains duplicate entries")
			}
			seenEntries[entryID] = struct{}{}
			isPrimary := 0
			if _, assigned := primary[entryID]; !assigned {
				isPrimary = 1
				primary[entryID] = struct{}{}
			}
			result, err := transaction.ExecContext(ctx, `
INSERT INTO entry_topics(user_id,entry_id,topic_id,confidence,is_primary,content_hash,created_at)
SELECT ?,e.entry_id,?,1,?,e.content_hash,?
FROM entries e JOIN account_entries ae ON ae.entry_id=e.entry_id
WHERE ae.user_id=? AND e.entry_id=?`, userID, topicID, isPrimary, timestamp, userID, entryID)
			if err != nil {
				return fmt.Errorf("assign generated topic: %w", err)
			}
			if affected, _ := result.RowsAffected(); affected != 1 {
				return errors.New("generated topic referenced an unknown entry")
			}
			newEntries = append(newEntries, entryID)
		}
	}
	if err := service.bumpTopicsRevisionTx(ctx, transaction, userID, timestamp); err != nil {
		return err
	}
	return service.indexer.RefreshTx(ctx, transaction, userID, uniqueEntryIDs(append(oldEntries, newEntries...)))
}

func topicEntryIDs(ctx context.Context, transaction *sql.Tx, userID string) ([]string, error) {
	rows, err := transaction.QueryContext(ctx, "SELECT DISTINCT entry_id FROM entry_topics WHERE user_id=?", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var entryID string
		if err := rows.Scan(&entryID); err != nil {
			return nil, err
		}
		result = append(result, entryID)
	}
	return result, rows.Err()
}

func uniqueEntryIDs(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
