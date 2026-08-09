package http

import (
	"bytes"
	"errors"
	"io/fs"
	"mime"
	stdhttp "net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const maximumStaticAssetBytes int64 = 256 * 1024 * 1024

var immutableAssetPattern = regexp.MustCompile(`(?:^|[.-])[a-f0-9]{8,}(?:[.-]|$)`)

type staticAsset struct {
	contents    []byte
	contentType string
	modifiedAt  time.Time
}

type spaHandler struct {
	assets map[string]staticAsset
	index  staticAsset
}

func NewSPAHandler(directory string) (stdhttp.Handler, error) {
	if directory == "" || !filepath.IsAbs(directory) {
		return nil, errors.New("static asset directory must be absolute")
	}
	rootInfo, err := os.Lstat(directory)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("static asset directory is unavailable")
	}
	handler := &spaHandler{assets: make(map[string]staticAsset)}
	var total int64
	err = filepath.WalkDir(directory, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return errors.New("read static assets failed")
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("static assets cannot contain symbolic links")
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() || info.Size() < 0 {
			return errors.New("static asset is invalid")
		}
		total += info.Size()
		if total > maximumStaticAssetBytes {
			return errors.New("static assets exceed the safe size limit")
		}
		contents, err := os.ReadFile(filePath)
		if err != nil {
			return errors.New("read static asset failed")
		}
		relative, err := filepath.Rel(directory, filePath)
		if err != nil {
			return errors.New("resolve static asset path failed")
		}
		name := filepath.ToSlash(relative)
		contentType := mime.TypeByExtension(filepath.Ext(name))
		if contentType == "" {
			contentType = stdhttp.DetectContentType(contents)
		}
		handler.assets[name] = staticAsset{contents: contents, contentType: contentType, modifiedAt: info.ModTime()}
		return nil
	})
	if err != nil {
		return nil, err
	}
	index, ok := handler.assets["index.html"]
	if !ok || len(index.contents) == 0 {
		return nil, errors.New("static index.html is required")
	}
	handler.index = index
	return handler, nil
}

func (handler *spaHandler) ServeHTTP(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
	if request.Method != stdhttp.MethodGet && request.Method != stdhttp.MethodHead {
		writer.WriteHeader(stdhttp.StatusMethodNotAllowed)
		return
	}
	name, ok := safeStaticPath(request.URL.Path)
	if !ok {
		writer.WriteHeader(stdhttp.StatusNotFound)
		return
	}
	asset, exists := handler.assets[name]
	if !exists {
		if path.Ext(name) != "" {
			writer.WriteHeader(stdhttp.StatusNotFound)
			return
		}
		name = "index.html"
		asset = handler.index
	}
	if name == "index.html" || name == "manifest.webmanifest" || name == "sw.js" {
		writer.Header().Set("Cache-Control", "no-store")
	} else if immutableAssetPattern.MatchString(path.Base(name)) {
		writer.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		writer.Header().Set("Cache-Control", "public, max-age=300")
	}
	writer.Header().Set("Content-Type", asset.contentType)
	stdhttp.ServeContent(writer, request, path.Base(name), asset.modifiedAt, bytes.NewReader(asset.contents))
}

func safeStaticPath(raw string) (string, bool) {
	if raw == "" || strings.ContainsAny(raw, "\\\x00") {
		return "", false
	}
	for _, segment := range strings.Split(strings.Trim(raw, "/"), "/") {
		if segment == "." || segment == ".." {
			return "", false
		}
	}
	cleaned := path.Clean("/" + strings.TrimPrefix(raw, "/"))
	if cleaned == "/" {
		return "index.html", true
	}
	name := strings.TrimPrefix(cleaned, "/")
	return name, fs.ValidPath(name) && name != "." && !strings.HasPrefix(name, "../")
}
