package handler

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMediaItemResponse_SeasonZero(t *testing.T) {
	zero := 0
	resp := MediaItemResponse{
		ID:      "test-id",
		Title:   "Special Episode",
		Season:  &zero,
		Episode: &zero,
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(data), `"season":0`) {
		t.Errorf("expected season:0 in JSON, got: %s", data)
	}
	if !strings.Contains(string(data), `"episode":0`) {
		t.Errorf("expected episode:0 in JSON, got: %s", data)
	}
}

func TestMediaItemResponse_NilSeason(t *testing.T) {
	resp := MediaItemResponse{
		ID:      "test-id",
		Title:   "A Movie",
		Season:  nil,
		Episode: nil,
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(string(data), `"season"`) {
		t.Errorf("expected no season in JSON for nil, got: %s", data)
	}
	if strings.Contains(string(data), `"episode"`) {
		t.Errorf("expected no episode in JSON for nil, got: %s", data)
	}
}

func TestMediaItemResponse_SeasonOne(t *testing.T) {
	one := 1
	ep := 5
	resp := MediaItemResponse{
		ID:      "test-id",
		Title:   "Episode 5",
		Season:  &one,
		Episode: &ep,
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(data), `"season":1`) {
		t.Errorf("expected season:1 in JSON, got: %s", data)
	}
	if !strings.Contains(string(data), `"episode":5`) {
		t.Errorf("expected episode:5 in JSON, got: %s", data)
	}
}
