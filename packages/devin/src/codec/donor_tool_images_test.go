package devin

import (
	"bytes"
	"testing"
)

func TestDonorDeferredToolImagesStayWithTheirRealResult(t *testing.T) {
	body := []byte(`{"model":"swe","messages":[{"role":"user","content":"look"},
		{"role":"assistant","tool_calls":[{"id":"c1","type":"function","function":{"name":"shot","arguments":"{}"}}]},
		{"role":"tool","tool_call_id":"c1","content":"done\n\n[The tool returned an image; it follows in the next message.]"},
		{"role":"user","content":[{"type":"text","text":"[From the result of shot (tool call c1):]"},
			{"type":"image_url","image_url":{"url":"data:image/png;base64,eA=="}},
			{"type":"text","text":"user words"},{"type":"image_url","image_url":{"url":"data:image/png;base64,eQ=="}}]}]}`)
	before := bytes.Clone(body)
	request, err := EncodeChatRequest(body, "selected-key")
	if err != nil {
		t.Fatal(err)
	}
	fields, _ := readMessage(request.Payload, nil)
	tool, _ := readMessage(fields[3][2].b, nil)
	user, _ := readMessage(fields[3][3].b, nil)
	if tool.text(3) != "done" || tool.text(7) != "c1" || len(tool[10]) != 1 ||
		user.text(3) != "user words" || len(user[10]) != 1 || !bytes.Equal(body, before) {
		t.Fatal("tool image ownership or user content changed")
	}
	toolImage, _ := readMessage(tool[10][0].b, nil)
	userImage, _ := readMessage(user[10][0].b, nil)
	if toolImage.text(1) != "eA==" || userImage.text(1) != "eQ==" {
		t.Fatal("images crossed owners")
	}
}

func TestDonorDeferredImageWrongCallRemainsUserOwnedAndIncompleteFails(t *testing.T) {
	missing := []byte(`{"model":"swe","messages":[{"role":"user","content":"look"},
		{"role":"assistant","tool_calls":[{"id":"c1","type":"function","function":{"name":"shot","arguments":"{}"}}]},
		{"role":"tool","tool_call_id":"c1","content":"[The tool returned 2 images; they follow in the next message.]"},
		{"role":"user","content":[{"type":"text","text":"[From the result of shot (tool call c1):]"},
			{"type":"image_url","image_url":{"url":"data:image/png;base64,eA=="}}]}]}`)
	if _, err := EncodeChatRequest(missing, "selected-key"); err == nil {
		t.Fatal("incomplete tool image association silently succeeded")
	}
	plain := []byte(`{"model":"swe","messages":[{"role":"user","content":[
		{"type":"text","text":"[From the result of tool call unrelated:]"},
		{"type":"image_url","image_url":{"url":"data:image/png;base64,eA=="}}]}]}`)
	result, err := EncodeChatRequest(plain, "selected-key")
	if err != nil {
		t.Fatal(err)
	}
	fields, _ := readMessage(result.Payload, nil)
	user, _ := readMessage(fields[3][0].b, nil)
	if user.number(2) != 1 || user.text(3) != "[From the result of tool call unrelated:]" || len(user[10]) != 1 {
		t.Fatal("unrelated user heading was consumed as a tool result")
	}
}
