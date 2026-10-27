package service

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

// Keep system/developer instructions separate from the first user message.
// Only text participates: image/audio payloads, tool calls, and assistant output
// must not create artificial prompt diversity.
func blackroomDistributionPrompts(request dto.Request) (system, firstUser []string) {
	switch r := request.(type) {
	case *dto.GeneralOpenAIRequest:
		if r == nil {
			return
		}
		firstUserSeen := false
		for _, message := range r.Messages {
			role := strings.ToLower(strings.TrimSpace(message.Role))
			if role != "system" && role != "developer" && (role != "user" || firstUserSeen) {
				continue
			}
			if role == "user" {
				firstUserSeen = true
			}
			text := message.StringContent()
			if text == "" {
				var parts []string
				for _, part := range message.ParseContent() {
					if part.Type == dto.ContentTypeText && strings.TrimSpace(part.Text) != "" {
						parts = append(parts, part.Text)
					}
				}
				text = strings.Join(parts, "\n")
			}
			if strings.TrimSpace(text) == "" {
				continue
			}
			if role == "user" {
				firstUser = []string{text}
			} else {
				system = append(system, role+":"+text)
			}
		}
	case *dto.ClaudeRequest:
		if r == nil {
			return
		}
		if text := r.GetStringSystem(); strings.TrimSpace(text) != "" {
			system = []string{text}
		} else {
			for _, part := range r.ParseSystem() {
				if part.Type == dto.ContentTypeText && strings.TrimSpace(part.GetText()) != "" {
					system = append(system, part.GetText())
				}
			}
		}
		for _, message := range r.Messages {
			if message.Role != "user" {
				continue
			}
			if text := message.GetStringContent(); strings.TrimSpace(text) != "" {
				firstUser = []string{text}
			} else if parts, err := message.ParseContent(); err == nil {
				for _, part := range parts {
					if part.Type == dto.ContentTypeText && strings.TrimSpace(part.GetText()) != "" {
						firstUser = append(firstUser, part.GetText())
					}
				}
			}
			break
		}
	case *dto.GeminiChatRequest:
		if r == nil {
			return
		}
		if r.SystemInstructions != nil {
			for _, part := range r.SystemInstructions.Parts {
				if strings.TrimSpace(part.Text) != "" {
					system = append(system, part.Text)
				}
			}
		}
		for _, content := range r.Contents {
			if content.Role != "" && content.Role != "user" {
				continue
			}
			for _, part := range content.Parts {
				if strings.TrimSpace(part.Text) != "" {
					firstUser = append(firstUser, part.Text)
				}
			}
			break
		}
	case *dto.OpenAIResponsesRequest:
		if r == nil {
			return
		}
		system = blackroomResponsesText(r.Instructions)
		// A delta with previous_response_id does not contain the conversation's
		// first user message. Counting its follow-ups as new starts is misleading.
		if r.PreviousResponseID != "" {
			return
		}
		if common.GetJsonType(r.Input) == "string" {
			firstUser = blackroomResponsesText(r.Input)
			return
		}
		var inputs []dto.Input
		if err := common.Unmarshal(r.Input, &inputs); err != nil {
			return
		}
		firstUserSeen := false
		for _, input := range inputs {
			if input.Type != "" && input.Type != "message" {
				continue
			}
			if input.Role == "system" || input.Role == "developer" {
				system = append(system, blackroomResponsesText(input.Content)...)
			} else if (input.Role == "user" || input.Role == "") && !firstUserSeen {
				firstUserSeen = true
				firstUser = blackroomResponsesText(input.Content)
			}
		}
	}
	return
}

func blackroomResponsesText(raw []byte) []string {
	if common.GetJsonType(raw) == "string" {
		var text string
		if common.Unmarshal(raw, &text) == nil && strings.TrimSpace(text) != "" {
			return []string{text}
		}
		return nil
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if common.Unmarshal(raw, &parts) != nil {
		return nil
	}
	var texts []string
	for _, part := range parts {
		if (part.Type == "text" || part.Type == "input_text") && strings.TrimSpace(part.Text) != "" {
			texts = append(texts, part.Text)
		}
	}
	return texts
}

func blackroomPromptFingerprint(userID, tokenID int, kind string, texts []string) string {
	if len(texts) == 0 || common.CryptoSecret == "" {
		return ""
	}
	data, err := common.Marshal(texts)
	if err != nil {
		return ""
	}
	// Full text, unambiguous framing, and per-account/key/kind domain separation.
	// HMAC prevents dictionary reversal of short/common prompts.
	scope := blackroomDistributionPrefix + strconv.Itoa(userID) + ":" + strconv.Itoa(tokenID) + ":" + kind + ":"
	return common.GenerateHMAC(scope + string(data))
}
