package devin

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
)

type GeminiThought struct {
	Signature string
	Format    string
	Model     string
}

type GeminiReply struct {
	text      strings.Builder
	thinking  strings.Builder
	signature strings.Builder
	format    string
	model     string
	tools     []NativeToolDelta
	byID      map[string]int
	bytes     int
}

func IsGeminiModel(model string) bool {
	return strings.Contains(strings.ToLower(model), "gemini")
}

func (reply *GeminiReply) Add(delta NativeDelta) error {
	reply.bytes += len(delta.Text) + len(delta.Thinking) + len(delta.Signature)
	reply.text.WriteString(delta.Text)
	reply.thinking.WriteString(delta.Thinking)
	reply.signature.WriteString(delta.Signature)
	if delta.SignatureType != "" {
		if delta.SignatureType != "gemini" || reply.format != "" && reply.format != delta.SignatureType {
			return errors.New("Gemini reply changed its actual signature type")
		}
		reply.format = delta.SignatureType
	}
	if delta.ActualModelUID != "" {
		if len(delta.ActualModelUID) > 512 || reply.model != "" && reply.model != delta.ActualModelUID {
			return errors.New("Gemini reply changed its generation model")
		}
		reply.model = delta.ActualModelUID
	}
	for _, change := range delta.Tools {
		reply.bytes += len(change.ID) + len(change.Name) + len(change.Arguments)
		if reply.byID == nil {
			reply.byID = map[string]int{}
		}
		index, found := reply.byID[change.ID]
		if change.ID == "" && len(reply.tools) > 0 {
			index, found = len(reply.tools)-1, true
		}
		if !found {
			if change.ID == "" || change.Name == "" || len(reply.tools) >= 256 {
				return errors.New("Gemini reply has no bounded tool identity")
			}
			index = len(reply.tools)
			reply.byID[change.ID] = index
			reply.tools = append(reply.tools, NativeToolDelta{ID: change.ID, Name: change.Name})
		}
		tool := &reply.tools[index]
		if change.Name != "" && tool.Name != change.Name {
			return errors.New("Gemini reply changed a tool name")
		}
		tool.Arguments += change.Arguments
	}
	if reply.bytes > 2<<20 {
		return errors.New("Gemini reply exceeds its retention limit")
	}
	return nil
}

func geminiThoughtKey(text, thinking string, tools []NativeToolDelta) (string, error) {
	normalized := make([]NativeToolDelta, len(tools))
	for index, tool := range tools {
		var arguments map[string]any
		decoder := json.NewDecoder(strings.NewReader(tool.Arguments))
		decoder.UseNumber()
		if decoder.Decode(&arguments) != nil || arguments == nil {
			return "", errors.New("Gemini reply has incomplete tool arguments")
		}
		encoded, err := json.Marshal(arguments)
		if err != nil {
			return "", err
		}
		normalized[index] = NativeToolDelta{ID: tool.ID, Name: tool.Name, Arguments: string(encoded)}
	}
	raw, err := json.Marshal(struct {
		Text     string
		Thinking string
		Tools    []NativeToolDelta
	}{text, thinking, normalized})
	if err != nil {
		return "", err
	}
	return SignatureDigest(raw), nil
}

func (reply *GeminiReply) Thought(selected string) (string, GeminiThought, error) {
	if reply.signature.Len() == 0 && reply.format == "" {
		return "", GeminiThought{}, nil
	}
	if !IsGeminiModel(selected) || reply.signature.Len() == 0 || reply.format != "gemini" {
		return "", GeminiThought{}, errors.New("Gemini reply has no complete actual signature")
	}
	key, err := geminiThoughtKey(reply.text.String(), reply.thinking.String(), reply.tools)
	model := reply.model
	if model == "" {
		model = selected
	}
	return key, GeminiThought{Signature: reply.signature.String(), Format: reply.format, Model: model}, err
}

func replaceOutboundPrompts(encoded EncodedChat, prompts [][]byte) (EncodedChat, error) {
	fields, err := RawFields(encoded.Payload)
	if err != nil {
		return EncodedChat{}, err
	}
	var payload []byte
	for number := 1; number <= 28; number++ {
		if number == 3 {
			for _, prompt := range prompts {
				payload = addBytes(payload, 3, prompt)
			}
		} else {
			payload = append(payload, fields[number]...)
		}
	}
	if len(payload) > maxBody {
		return EncodedChat{}, errors.New("signed native request exceeds its byte limit")
	}
	encoded.Payload = payload
	return encoded, nil
}

func RestoreGeminiThoughts(encoded EncodedChat, lookup func(string) (GeminiThought, bool)) (EncodedChat, []NativeClaim, error) {
	if !IsGeminiModel(encoded.Model) || lookup == nil {
		return encoded, nil, nil
	}
	fields, err := readMessage(encoded.Payload, nil)
	if err != nil {
		return EncodedChat{}, nil, err
	}
	prompts := make([][]byte, len(fields[3]))
	var claims []NativeClaim
	for index := 0; index < len(fields[3]); {
		start := index
		var text, thinking strings.Builder
		var tools []NativeToolDelta
		target := -1
		explicit := false
		for index < len(fields[3]) {
			prompt, err := readMessage(fields[3][index].b, nil)
			if err != nil {
				return EncodedChat{}, nil, err
			}
			prompts[index] = fields[3][index].b
			if prompt.number(2) != 2 {
				break
			}
			text.WriteString(prompt.text(3))
			thinking.WriteString(prompt.text(11))
			explicit = explicit || prompt.text(12) != ""
			for _, item := range prompt[6] {
				tool, err := readMessage(item.b, nil)
				if err != nil {
					return EncodedChat{}, nil, err
				}
				tools = append(tools, NativeToolDelta{ID: tool.text(1), Name: tool.text(2), Arguments: tool.text(3)})
				target = index
			}
			index++
		}
		if index == start {
			index++
			continue
		}
		if explicit {
			continue
		}
		key, err := geminiThoughtKey(text.String(), thinking.String(), tools)
		if err != nil {
			return EncodedChat{}, nil, err
		}
		thought, found := lookup(key)
		if !found {
			continue
		}
		if thought.Signature == "" || thought.Format != "gemini" || thought.Model == "" || len(thought.Model) > 512 {
			return EncodedChat{}, nil, errors.New("Gemini continuation has no complete scoped signature")
		}
		if target < 0 {
			target = index - 1
		}
		prompts[target] = addText(addText(addText(bytes.Clone(prompts[target]), 12, thought.Signature), 18, thought.Format), 22, thought.Model)
		claims = append(claims, NativeClaim{Kind: "signature", Digest: SignatureDigest([]byte(thought.Signature)),
			Format: thought.Format, Model: thought.Model})
	}
	if len(claims) == 0 {
		return encoded, nil, nil
	}
	encoded, err = replaceOutboundPrompts(encoded, prompts)
	return encoded, claims, err
}

// encodeChatDetails keeps each scoped signature attached to its own thinking
// prompt. A Chat message may contain several native Claude thinking blocks.
func encodeChatDetails(entry outboundMessage, prompt []byte, model string, resolve SignatureResolver) ([][]byte, error) {
	if len(entry.ReasoningDetails) == 0 {
		return [][]byte{prompt}, nil
	}
	gemini := IsGeminiModel(model)
	if entry.Role != "assistant" || !gemini && !isClaudeModel(model) || len(entry.ReasoningDetails) > 256 || gemini && len(entry.ReasoningDetails) != 1 || resolve == nil {
		return nil, errors.New("signed Chat history requires a scoped native origin")
	}
	var thinking strings.Builder
	var prompts [][]byte
	for index, detail := range entry.ReasoningDetails {
		if detail.Type != "reasoning.text" || detail.Index != index || detail.Format == "" || len(detail.Format) > 256 ||
			gemini && detail.Format != "gemini" || !gemini && detail.Format == "gemini" || detail.Signature == "" {
			return nil, errors.New("unsupported or inconsistent signed Chat reasoning")
		}
		text := detail.Text
		if len(entry.ReasoningDetails) == 1 && text == "" {
			text = entry.ReasoningContent
		}
		thinking.WriteString(text)
		origin, err := resolve(detail.Signature)
		if err != nil {
			return nil, err
		}
		if origin.Format != detail.Format || gemini && origin.Model == "" || len(origin.Model) > 512 {
			return nil, errors.New("signed Chat history changed its actual signature origin")
		}
		block := prompt
		if len(entry.ReasoningDetails) > 1 {
			block, err = encodeOutboundMessage(outboundMessage{Role: "assistant", ReasoningContent: text})
			if err != nil {
				return nil, err
			}
		}
		prompts = append(prompts, addText(addText(addText(block, 12, detail.Signature), 18, origin.Format), 22, origin.Model))
	}
	if thinking.String() != entry.ReasoningContent {
		return nil, errors.New("unsupported or inconsistent signed Chat reasoning")
	}
	if len(entry.ReasoningDetails) > 1 {
		entry.ReasoningContent = ""
		tail, err := encodeOutboundMessage(entry)
		if err != nil {
			return nil, err
		}
		prompts = append(prompts, tail)
	}
	return prompts, nil
}
