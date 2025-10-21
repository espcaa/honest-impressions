package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/joho/godotenv"
)

type HonestImpressionsBot struct {
	Router     *chi.Mux
	Params     BotParams
	SlackToken string
}

type BotParams struct {
	Port string
}

func NewHonestImpressionsBot() *HonestImpressionsBot {

	port := os.Getenv("PORT")

	if port == "" {
		port = "8080"
	}

	slackToken := os.Getenv("SLACK_BOT_TOKEN")
	if slackToken == "" {
		log.Fatal("SLACK_BOT_TOKEN environment variable is required")
	}

	return &HonestImpressionsBot{
		Params: BotParams{
			Port: port,
		},
		SlackToken: slackToken,
	}
}

func (bot *HonestImpressionsBot) Start() {
	// setup a chi router

	r := chi.NewRouter()
	bot.Router = r

	// handle an api route to check health

	bot.Router.Get("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("^-^"))
	})

	bot.Router.Post("/api/new-impression", func(w http.ResponseWriter, r *http.Request) {
		log.Println("received new impression request")
		bot.HandleNewImpression(w, r)
	})

	log.Println("starting server on port " + bot.Params.Port)
	// start the server
	http.ListenAndServe(":"+bot.Params.Port, bot.Router)

}

func main() {
	log.Println("starting...")

	godotenv.Load()
	bot := NewHonestImpressionsBot()
	bot.Start()
}

func (bot *HonestImpressionsBot) HandleNewImpression(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "failed to parse form", http.StatusBadRequest)
		return
	}

	payload := r.PostFormValue("payload")
	if payload == "" {
		http.Error(w, "missing payload", http.StatusBadRequest)
		return
	}

	var data struct {
		Type      string `json:"type"`
		TriggerID string `json:"trigger_id"`
		View      struct {
			ID    string `json:"id"`
			State struct {
				Values map[string]map[string]struct {
					Value string `json:"value"`
				} `json:"values"`
			} `json:"state"`
		} `json:"view"`
	}

	if err := json.Unmarshal([]byte(payload), &data); err != nil {
		http.Error(w, "invalid JSON payload", http.StatusBadRequest)
		return
	}

	switch data.Type {
	case "shortcut":
		err := bot.OpenImpressionModal(data.TriggerID)
		if err != nil {
			log.Printf("Failed to open modal: %v", err)
			http.Error(w, "failed to open modal", http.StatusInternalServerError)
			return
		}
	case "view_submission":
		impression := data.View.State.Values["impression_input"]["impression_value"].Value
		log.Printf("New honest impression : %s", impression)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`))
		return
	default:
		http.Error(w, "unsupported interaction type", http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (bot *HonestImpressionsBot) OpenImpressionModal(triggerID string) error {
	view := map[string]any{
		"type": "modal",
		"title": map[string]string{
			"type": "plain_text",
			"text": "So you're here to give an honest impression?",
		},
		"submit": map[string]string{
			"type": "plain_text",
			"text": "Submit",
		},
		"close": map[string]string{
			"type": "plain_text",
			"text": "Cancel",
		},
		"blocks": []map[string]any{
			{
				"type":     "input",
				"block_id": "impression_input",
				"label": map[string]string{
					"type": "plain_text",
					"text": "nice... please be honest (but not rude) and keep in mind that even though you're anon, this will be reviewed!",
				},
				"element": map[string]any{
					"type":      "plain_text_input",
					"action_id": "impression_value",
					"multiline": true,
					"placeholder": map[string]string{
						"type": "plain_text",
						"text": "[impression here?]",
					},
				},
			},
		},
	}

	payload := map[string]any{
		"trigger_id": triggerID,
		"view":       view,
	}

	body, _ := json.Marshal(payload)

	req, _ := http.NewRequest("POST", "https://slack.com/api/views.open", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+bot.SlackToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("slack API error: %s", string(b))
	}

	return nil
}
