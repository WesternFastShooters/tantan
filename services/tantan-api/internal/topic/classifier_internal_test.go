package topic

import "testing"

func TestTinyFilteredCorpusDoesNotInventNavigationLabels(t *testing.T) {
	documents := []classificationDocument{
		{ID: "entry_1", Title: "RT SpaceX: Falcon 9 launches 24 Starlink satellites from California", Body: "Watch Falcon 9 launch", Source: "Twitter", Weights: map[string]float64{}, Labels: map[string]string{}},
		{ID: "entry_2", Title: "SpaceX Falcon 9 launches Starlink satellites", Body: "Falcon 9 mission from California", Source: "Twitter", Weights: map[string]float64{}, Labels: map[string]string{}},
	}
	for index := range documents {
		addTerms(documents[index].Weights, documents[index].Labels, documents[index].Title, 3)
		addTerms(documents[index].Weights, documents[index].Labels, documents[index].Body, 1)
	}
	set := classifyDocuments(documents)
	if len(set.Topics) != 1 || set.Topics[0].Name != "航天" || len(set.Topics[0].EntryIDs) != 2 {
		t.Fatalf("topics=%#v", set.Topics)
	}
}
