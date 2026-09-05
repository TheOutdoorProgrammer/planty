package judge

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestPhotoCatalogExplainsHowToOpenEachImage(t *testing.T) {
	offers := []Offer{{Label: "latest, sent with message 3"}, {Label: "previous"}}
	messages, err := messagesForChat(Request{
		Turns: []Turn{ask(text("Look at the latest image."))}, Offered: offers,
	})
	if err != nil {
		t.Fatal(err)
	}
	catalog := messages[len(messages)-1].Content.(string)
	for _, want := range []string{"historical_photo", `{"index":0}`, "0: latest, sent with message 3", "1: previous"} {
		if !strings.Contains(catalog, want) {
			t.Errorf("catalog is missing %q: %s", want, catalog)
		}
	}
	if strings.Contains(catalog, "Say so if one would have changed your answer") {
		t.Error("the model was told to disclaim photos it can open")
	}
	definition := newToolbox(nil, offers...).definitions()[0]
	if definition.Function.Description != catalog {
		t.Error("the prompt and photo tool describe different catalogues")
	}
}

func TestPhotoArgumentErrorsAllowRecovery(t *testing.T) {
	for _, tc := range []struct {
		name, args, want string
	}{
		{"missing", `{}`, "Missing required integer argument index"},
		{"wrong name", `{"photo_index":0}`, "Missing required integer argument index"},
		{"null", `{"index":null}`, "Missing required integer argument index"},
		{"negative", `{"index":-1}`, "available range 0 through 0"},
		{"too large", `{"index":1}`, "available range 0 through 0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			box := newToolbox(nil, Offer{Label: "today", Media: "image/png", Bytes: testImage(t, "png")})
			call := toolCall{}
			call.Function.Name = "historical_photo"
			call.Function.Arguments = tc.args
			result := box.run(context.Background(), call)
			if !strings.Contains(result.Content, tc.want) || !strings.Contains(result.Content, `{"index":0}`) {
				t.Errorf("unhelpful photo error: %s", result.Content)
			}
			if len(result.Images) != 0 {
				t.Error("an invalid index returned image content")
			}
			call.Function.Arguments = `{"index":0}`
			if result := box.run(context.Background(), call); len(result.Images) != 2 || result.Summary != "Opened today" {
				t.Fatalf("corrected call did not open the photo: %+v", result)
			}
		})
	}
}

func TestPhotoToolWithoutOffersReportsUnavailable(t *testing.T) {
	call := toolCall{}
	call.Function.Name = "historical_photo"
	call.Function.Arguments = `{"index":0}`
	result := newToolbox(nil).run(context.Background(), call)
	if result.Content != "No photographs are available to open in this request." || len(result.Images) != 0 {
		t.Fatalf("empty catalogue result: %+v", result)
	}
}

func TestParallelPhotoResultsUseUserImagesAfterEveryToolReply(t *testing.T) {
	call := `{"choices":[{"message":{"role":"assistant","tool_calls":[
		{"id":"photo-1","type":"function","function":{"name":"historical_photo","arguments":"{\"index\":0}"}},
		{"id":"denied","type":"function","function":{"name":"planty_agent","arguments":"{\"command\":\"planty agent today\"}"}},
		{"id":"photo-2","type":"function","function":{"name":"historical_photo","arguments":"{\"index\":1}"}}
	]}}]}`
	backend, seen := serve(t, call, replied(t, "The photos are green."), replied(t, `{"answer":"green"}`))
	_, err := backend.Judge(context.Background(), Request{
		Turns: []Turn{ask(text("Compare these photos."))},
		Offered: []Offer{
			{Label: "first", Media: "image/png", Bytes: testImage(t, "png")},
			{Label: "second", Media: "image/jpeg", Bytes: testImage(t, "jpeg")},
		},
		Schema: probeSchema,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, request := range (*seen)[1:] {
		resolved, photos := 0, 0
		for _, message := range request.Messages {
			if message.Role == "tool" {
				if _, ok := message.Content.(string); !ok {
					t.Fatalf("tool reply must contain text: %+v", message)
				}
				if message.ToolCallID != []string{"photo-1", "denied", "photo-2"}[resolved] {
					t.Fatalf("tool reply lost its call ID: %+v", message)
				}
				resolved++
			}
			raw, err := json.Marshal(message.Content)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(raw), "image_url") {
				if message.Role != "user" || resolved != 3 {
					t.Fatal("images interrupted unresolved tool calls or used the wrong role")
				}
				photos += strings.Count(string(raw), "data:image/")
				for _, label := range []string{"historical_photo index 0: first", "historical_photo index 1: second"} {
					if !strings.Contains(string(raw), label) {
						t.Errorf("image lost its identity: %s", label)
					}
				}
			}
		}
		if resolved != 3 || photos != 2 {
			t.Fatalf("request has %d tool results and %d images", resolved, photos)
		}
	}
}
