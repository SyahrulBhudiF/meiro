package youtube

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestRemoveFromPlaylistValidatesAuthenticationAndIdentifiers(t *testing.T) {
	client := NewClient(Options{})
	for _, test := range []struct {
		name, playlist, entry string
	}{
		{"signed out", "PL1", "entry-1"},
		{"missing playlist", "", "entry-1"},
		{"missing entry", "PL1", " "},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := client.RemoveFromPlaylist(context.Background(), test.playlist, test.entry); err == nil {
				t.Fatal("invalid removal was accepted")
			}
		})
	}
}

type playlistResponse string

func (p playlistResponse) RoundTrip(request *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(string(p))),
		Request:    request,
	}, nil
}

func TestRemoveFromPlaylistSendsOccurrenceID(t *testing.T) {
	auth, err := NewCookieAuth("SAPISID=test", CookieOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	client := NewClient(Options{
		CookieAuth: auth,
		HTTPClient: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			if err := json.NewDecoder(request.Body).Decode(&got); err != nil {
				t.Fatal(err)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"status":"STATUS_SUCCEEDED"}`)),
				Request:    request,
			}, nil
		})},
	})
	if err := client.RemoveFromPlaylist(context.Background(), "VLPL1", "entry-1"); err != nil {
		t.Fatal(err)
	}
	if got["playlistId"] != "PL1" {
		t.Fatalf("playlist ID = %#v", got["playlistId"])
	}
	actions, ok := got["actions"].([]any)
	if !ok || len(actions) != 1 {
		t.Fatalf("actions = %#v", got["actions"])
	}
	action, ok := actions[0].(map[string]any)
	if !ok || action["action"] != "ACTION_REMOVE_VIDEO" || action["setVideoId"] != "entry-1" {
		t.Fatalf("action = %#v", actions[0])
	}
}

func TestRemoveFromPlaylistRefusal(t *testing.T) {
	auth, err := NewCookieAuth("SAPISID=test", CookieOptions{})
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient(Options{
		CookieAuth: auth,
		HTTPClient: &http.Client{Transport: playlistResponse(`{"status":"STATUS_INVALID_PLAYLIST_SET_VIDEO_ID"}`)},
	})
	if err := client.RemoveFromPlaylist(context.Background(), "VLPL1", "entry-1"); err == nil || !strings.Contains(err.Error(), "STATUS_INVALID_PLAYLIST_SET_VIDEO_ID") {
		t.Fatalf("refusal = %v", err)
	}
}
