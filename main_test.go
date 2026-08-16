package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestMediaServerRejectsUnauthorisedFile(t *testing.T) {
	server := newMediaServer()
	req := httptest.NewRequest(http.MethodGet, "/media?path=/tmp/not-allowed.mp4", nil)
	res := httptest.NewRecorder()
	server.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("expected %d, got %d", http.StatusForbidden, res.Code)
	}
}

func TestMediaServerSupportsRangeRequests(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.mp4")
	if err := os.WriteFile(path, []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := newMediaServer()
	server.allow(path)
	req := httptest.NewRequest(http.MethodGet, "/media?path="+path, nil)
	req.Header.Set("Range", "bytes=2-5")
	res := httptest.NewRecorder()
	server.ServeHTTP(res, req)
	if res.Code != http.StatusPartialContent || res.Body.String() != "2345" {
		t.Fatalf("unexpected range response: status=%d body=%q", res.Code, res.Body.String())
	}
}

func TestMediaKind(t *testing.T) {
	if got := mediaKind("track.FLAC"); got != "audio" {
		t.Fatalf("expected audio, got %q", got)
	}
	if got := mediaKind("movie.mp4"); got != "video" {
		t.Fatalf("expected video, got %q", got)
	}
}

func TestDeleteVideoRequiresAuthorisation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private.mp4")
	if err := os.WriteFile(path, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	app := NewApp(newMediaServer())
	if err := app.DeleteVideo(path); err == nil {
		t.Fatal("expected unauthorised deletion to fail")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("unauthorised file was removed")
	}
}

func TestDeleteVideoRemovesAuthorisedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "delete-me.mp4")
	if err := os.WriteFile(path, []byte("video"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := newMediaServer()
	server.allow(path)
	app := NewApp(server)
	if err := app.DeleteVideo(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("authorised file still exists")
	}
	if server.isAllowed(path) {
		t.Fatal("deleted file remained authorised")
	}
}

func TestRecentStorePersistsNewestFirstAndDeduplicates(t *testing.T) {
	store := &recentStore{path: filepath.Join(t.TempDir(), "recent.json")}
	first := filepath.Join(t.TempDir(), "first.mp4")
	second := filepath.Join(t.TempDir(), "second.mp4")
	if err := store.add(first); err != nil {
		t.Fatal(err)
	}
	if err := store.add(second); err != nil {
		t.Fatal(err)
	}
	if err := store.add(first); err != nil {
		t.Fatal(err)
	}
	paths, err := store.list()
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || paths[0] != first || paths[1] != second {
		t.Fatalf("unexpected recent order: %#v", paths)
	}
}
