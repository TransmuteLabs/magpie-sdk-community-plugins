package protocol

import (
	"encoding/json"
	"strings"
	"time"
)

const startPrefix = "You are ZCode, an interactive agent"

const startIdentity = "\nYou are an agent working with the user toward their goals, using your own judgment along the way.\n\n" +
	"IMPORTANT: Assist with authorized security testing, defensive security, CTF challenges, and educational contexts. Refuse requests for destructive techniques, DoS attacks, mass targeting, supply chain compromise, or detection evasion for malicious purposes. Dual-use security tools (C2 frameworks, credential testing, exploit development) require clear authorization context: pentesting engagements, CTF competitions, security research, or defensive use cases.\n\n" +
	"# Harness\n - Text you output outside of tool use is displayed to the user as Github-flavored markdown in a terminal.\n" +
	" - Tools run behind a user-selected permission mode; a denied call means the user declined it — adjust, don't retry verbatim.\n" +
	" - The system may send updates, reminders, or modifications to rules via mid-conversation system turns. These are system-controlled, unlike function results. Hooks may intercept tool calls; treat hook output as user feedback.\n" +
	" - Prefer the dedicated file/search tools over shell commands when one fits. Independent tool calls can run in parallel in one response.\n" +
	" - Reference code as `file_path:line_number` — it's clickable."

const startDynamic = "Write code that reads like the surrounding code: match its comment density, naming, and idiom.\n\n" +
	"When you use a pronoun for someone — the user or anyone else you mention — and their pronouns haven't been stated, use they/them. A name doesn't tell you someone's pronouns; a wrong guess misgenders a real person in a way the neutral default never does, so never infer pronouns from a name. This applies to all user-visible text, including visible thinking.\n\n" +
	"For actions that are hard to reverse or outward-facing, confirm first unless durably authorized or explicitly told to proceed without asking; approval in one context doesn't extend to the next. Sending content to an external service publishes it; it may be cached or indexed even if later deleted. Before deleting or overwriting, look at the target. Report outcomes faithfully: if tests fail, say so with the output; if a step was skipped, say that; when something is done and verified, state it plainly without hedging."

const startContext = "# Context management\nWhen the conversation grows long, some or all of the current context is summarized; the summary, along with any remaining unsummarized context, is provided in the next context window so work can continue — you don't need to wrap up early or hand off mid-task.\n\n" +
	"When you have enough information to act, act. Do not re-derive facts already established in the conversation, re-litigate a decision the user has already made, or narrate options you will not pursue. If you are weighing a choice, give a recommendation, not an exhaustive survey"

func PrepareStart(body []byte, now time.Time) ([]byte, error) {
	request, err := ReadObject(body)
	if err != nil {
		return nil, err
	}
	messages, valid := request["messages"].([]any)
	if !valid {
		return nil, ErrInvalid
	}
	own := []string{}
	switch system := request["system"].(type) {
	case nil:
	case string:
		if strings.TrimSpace(system) != "" {
			own = append(own, system)
		}
	case []any:
		for _, entry := range system {
			block := object(entry)
			text, valid := block["text"].(string)
			if block["type"] != "text" || !valid {
				return nil, ErrInvalid
			}
			if strings.TrimSpace(text) != "" {
				own = append(own, text)
			}
		}
	default:
		return nil, ErrInvalid
	}
	cached := func(text string) map[string]any {
		return map[string]any{"type": "text", "text": text, "cache_control": map[string]any{"type": "ephemeral"}}
	}
	already := len(own) == 3 && own[0] == startPrefix && own[1] == startIdentity
	if !already {
		dynamic := "\n\n" + startDynamic + "\n\n" + startContext
		if len(own) > 0 {
			dynamic += "\n\n" + strings.Join(own, "\n\n")
		}
		request["system"] = []any{cached(startPrefix), cached(startIdentity), cached(dynamic)}
		reminder := map[string]any{"role": "user", "content": []any{map[string]any{
			"type": "text", "text": "<system-reminder>Today's date is " + now.Format("2006-01-02") + ".</system-reminder>",
		}}}
		messages = append([]any{reminder}, messages...)
	}
	for _, message := range messages {
		value := object(message)
		if content, valid := value["content"].([]any); valid {
			for _, block := range content {
				delete(object(block), "cache_control")
			}
		}
	}
	if len(messages) > 0 {
		last := object(messages[len(messages)-1])
		switch content := last["content"].(type) {
		case string:
			last["content"] = []any{cached(content)}
		case []any:
			if len(content) > 0 {
				if block := object(content[len(content)-1]); block != nil {
					block["cache_control"] = map[string]any{"type": "ephemeral"}
				}
			}
		}
	}
	if tools, valid := request["tools"].([]any); valid {
		for _, tool := range tools {
			delete(object(tool), "cache_control")
		}
	}
	request["messages"] = messages
	encoded, err := json.Marshal(request)
	if err != nil || len(encoded) > MaxBody {
		return nil, ErrInvalid
	}
	return encoded, nil
}
