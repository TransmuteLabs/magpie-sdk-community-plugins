package devin

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"unicode/utf8"

	"github.com/yetone/magpie/pkg/protocolext"
)

type AccountKind string

const (
	UserStatus          AccountKind = "user-status"
	TeamSettings        AccountKind = "team-settings"
	ManagedPlugins      AccountKind = "managed-plugins"
	ManagedPluginBundle AccountKind = "managed-plugin-bundle"
	ModelAssignment     AccountKind = "model-assignment"
)

func (kind AccountKind) Path() string {
	switch kind {
	case UserStatus:
		return "/exa.seat_management_pb.SeatManagementService/GetUserStatus"
	case TeamSettings:
		return "/exa.seat_management_pb.SeatManagementService/GetCliTeamSettings"
	case ManagedPlugins:
		return "/exa.api_server_pb.ApiServerService/GetAccountManagedPlugins"
	case ManagedPluginBundle:
		return "/exa.api_server_pb.ApiServerService/GetAccountManagedPluginBundle"
	case ModelAssignment:
		return "/exa.api_server_pb.ApiServerService/AssignModel"
	}
	return ""
}

type AccountAdapter struct{ Kind AccountKind }

func (adapter AccountAdapter) ID() string             { return "devin.account." + string(adapter.Kind) }
func (adapter AccountAdapter) Path() string           { return adapter.Kind.Path() }
func (AccountAdapter) Contract() protocolext.Contract { return protocolext.AccountOperationContract() }

func (adapter AccountAdapter) Decode(ctx context.Context, header http.Header, body []byte) (protocolext.Request, error) {
	if len(body) > 1<<20 {
		return protocolext.Request{}, refusal(413, "native account request exceeds its byte limit")
	}
	if adapter.Path() == "" {
		return protocolext.Request{}, refusal(400, "unknown native account operation")
	}
	schema := map[int]int{1: 2}
	switch adapter.Kind {
	case ManagedPluginBundle:
		schema = map[int]int{1: 2, 2: textField, 3: 0}
	case ModelAssignment:
		schema = map[int]int{1: 2, 2: textField, 3: textField, 5: 2, 6: textField}
	}
	fields, err := readMessage(body, schema)
	if err != nil {
		return protocolext.Request{}, refusal(400, "invalid native account operation request")
	}
	if err := validateAccountMetadata(fields.data(1)); err != nil {
		return protocolext.Request{}, refusal(400, "invalid native account credential metadata")
	}
	authority := addBytes(nil, 1, fields.data(1))
	decoded, err := (CatalogAdapter{Kind: CliModels}).Decode(ctx, header, authority)
	if err != nil {
		return protocolext.Request{}, err
	}
	decoded.Protocol, decoded.Executor = protocolext.AccountOperation, protocolext.SameModuleExecutor
	decoded.AccountOperation = string(adapter.Kind)
	switch adapter.Kind {
	case ManagedPluginBundle:
		decoded.Body, err = bundleSelection(fields)
		if err != nil {
			return protocolext.Request{}, refusal(400, "invalid native managed bundle selection")
		}
	case ModelAssignment:
		if fields.text(2) == "" || fields.text(3) == "" {
			return protocolext.Request{}, refusal(400, "native assignment needs its model router and cascade identity")
		}
		clean, err := RawFields(body)
		if err != nil {
			return protocolext.Request{}, refusal(400, "invalid native assignment envelope")
		}
		for _, number := range []int{2, 3, 5, 6} {
			decoded.Body = append(decoded.Body, clean[number]...)
		}
		decoded.Session = fields.text(3)
		decoded.RequestID = "assignment-" + SignatureDigest(decoded.Body)
	}
	return decoded, nil
}

func bundleSelection(fields message) ([]byte, error) {
	bundleID, scope := fields.text(2), fields.number(3)
	if bundleID == "" || len(bundleID) > 128 || bundleID == "." || bundleID == ".." || scope < 1 || scope > 3 {
		return nil, errors.New("invalid native managed bundle selection")
	}
	for _, char := range bundleID {
		if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' ||
			char == '-' || char == '.' || char == '_') {
			return nil, errors.New("invalid native managed bundle selection")
		}
	}
	return addNumber(addText(nil, 2, bundleID), 3, scope), nil
}

func validateAccountMetadata(payload []byte) error {
	fields, err := readMessage(payload, nil)
	if err != nil {
		return err
	}
	for _, display := range fields[30] {
		if display.w == 0 {
			if display.u > 1<<31-1 {
				return errors.New("invalid native display hint")
			}
			continue
		}
		if display.w != 2 || len(display.b) > 1024 {
			return errors.New("invalid native display hints")
		}
		packed := display.b
		for len(packed) > 0 {
			value, size := binary.Uvarint(packed)
			if size <= 0 || value > 1<<31-1 {
				return errors.New("invalid native display hints")
			}
			packed = packed[size:]
		}
	}
	if values := fields[31]; len(values) > 1 ||
		len(values) == 1 && (values[0].w != 2 || len(values[0].b) > 64<<10 || !utf8.Valid(values[0].b)) {
		return errors.New("invalid native fingerprint metadata")
	}
	chunks, err := RawFields(payload)
	if err != nil {
		return err
	}
	numbers := make([]int, 0, len(chunks))
	for number := range chunks {
		numbers = append(numbers, number)
	}
	slices.Sort(numbers)
	var remaining []byte
	for _, number := range numbers {
		if number != 30 && number != 31 {
			remaining = append(remaining, chunks[number]...)
		}
	}
	return validateNativeMetadata(remaining)
}

func AccountPayload(kind AccountKind, body []byte, model, key string) ([]byte, error) {
	metadata, err := NativeMetadata(key)
	if err != nil || kind.Path() == "" {
		return nil, errors.New("invalid native account operation authority")
	}
	if kind == ManagedPluginBundle {
		fields, err := readMessage(body, map[int]int{2: textField, 3: 0})
		if err != nil {
			return nil, err
		}
		selection, err := bundleSelection(fields)
		if err != nil {
			return nil, err
		}
		return append(addBytes(nil, 1, metadata), selection...), nil
	}
	if kind != ModelAssignment {
		if len(body) != 0 {
			return nil, errors.New("native account read accepts no client authority")
		}
		return addBytes(nil, 1, metadata), nil
	}
	fields, err := readMessage(body, map[int]int{2: textField, 3: textField, 5: 2, 6: textField})
	if err != nil || fields.text(2) != model || fields.text(3) == "" {
		return nil, errors.New("native assignment must use the host-selected model router")
	}
	if fields.text(6) != "" && fields.text(6) != model {
		return nil, errors.New("native assignment cannot choose another lead model router")
	}
	if len(fields.data(5)) > 0 {
		projection := addText(nil, 21, model)
		projection = addBytes(projection, 3, fields.data(5))
		wire, compressed, err := EncodeConnectRequest(projection)
		if err != nil {
			return nil, err
		}
		headers := http.Header{"Content-Type": {"application/connect+proto"}, "Connect-Protocol-Version": {"1"}}
		if compressed {
			headers.Set("Connect-Content-Encoding", "gzip")
		}
		envelope, err := ParseNativeEnvelope(context.Background(), headers, wire)
		if err != nil || len(envelope.Claims) != 0 {
			return nil, errors.New("native assignment prompt cannot import unproven signed history")
		}
	}
	return append(addBytes(nil, 1, metadata), body...), nil
}

func ValidateAccountResponse(kind AccountKind, body []byte) error {
	if len(body) > 1<<20 {
		return errors.New("native account response exceeds its byte limit")
	}
	if kind == ManagedPluginBundle {
		bundle, err := projectedMessage(body, map[int]int{1: 0, 2: textField, 3: textField, 4: 2 | repeated})
		if err != nil || bundle.number(1) > 1 {
			return errors.New("invalid native managed bundle response")
		}
		for _, header := range bundle[4] {
			if _, err := projectedMessage(header.b, map[int]int{1: textField, 2: textField}); err != nil {
				return errors.New("invalid native managed bundle header")
			}
		}
		return nil
	}
	fields, err := readMessage(body, nil)
	if err != nil {
		return errors.New("invalid native account response")
	}
	switch kind {
	case UserStatus:
		fields, err = projectedMessage(body, map[int]int{1: 2, 2: 2})
		if err != nil {
			return errors.New("invalid native account status response")
		}
		for _, number := range []int{1, 2} {
			if len(fields[number]) > 1 {
				return errors.New("ambiguous native account status")
			}
			if _, err := readMessage(fields.data(number), nil); err != nil {
				return errors.New("invalid native account status")
			}
		}
	case TeamSettings:
	case ManagedPlugins:
		fields, err = projectedMessage(body, map[int]int{1: textField, 2: 0, 3: textField,
			4: textField, 5: 0, 6: textField, 7: textField, 8: 0, 9: 0})
		if err != nil {
			return errors.New("invalid native managed plugin response")
		}
		for _, number := range []int{1, 4, 7} {
			if len(fields[number]) > 1 || len(fields.data(number)) > 0 && !json.Valid(fields.data(number)) {
				return errors.New("invalid native managed plugin manifest")
			}
		}
	case ModelAssignment:
		if _, err := DecodeAssignment(body); err != nil {
			return err
		}
	default:
		return errors.New("unknown native account response")
	}
	return nil
}

type Assignment struct{ Token, Model string }

func AssignmentCascade(body []byte) (string, error) {
	fields, err := readMessage(body, map[int]int{2: textField, 3: textField, 5: 2, 6: textField})
	if err != nil || fields.text(3) == "" {
		return "", errors.New("invalid native assignment cascade")
	}
	return fields.text(3), nil
}

func DecodeAssignment(body []byte) (Assignment, error) {
	envelope, err := readMessage(body, map[int]int{1: 2})
	if err != nil {
		return Assignment{}, errors.New("invalid native assignment response")
	}
	fields, err := readMessage(envelope.data(1), map[int]int{1: textField, 2: textField, 3: textField | repeated})
	if err != nil || fields.text(1) == "" || fields.text(2) == "" {
		return Assignment{}, errors.New("native assignment has no token or model")
	}
	return Assignment{Token: fields.text(1), Model: fields.text(2)}, nil
}

func (AccountAdapter) Reply(writer http.ResponseWriter) protocolext.Response {
	return &accountResponse{writer: writer, header: make(http.Header)}
}

type accountResponse struct {
	writer   http.ResponseWriter
	header   http.Header
	status   int
	body     bytes.Buffer
	finished bool
}

func (reply *accountResponse) Header() http.Header { return reply.header }
func (reply *accountResponse) WriteHeader(status int) {
	if !reply.finished && reply.status == 0 {
		reply.status = status
	}
}
func (reply *accountResponse) Write(body []byte) (int, error) {
	if reply.finished {
		return 0, io.ErrClosedPipe
	}
	if reply.body.Len()+len(body) > 1<<20 {
		return 0, errors.New("native account response exceeds its byte limit")
	}
	if reply.status == 0 {
		reply.status = http.StatusOK
	}
	return reply.body.Write(body)
}
func (reply *accountResponse) Finish(cause error) error {
	if reply.finished {
		return nil
	}
	reply.finished = true
	if cause != nil {
		reply.status = 500
		if errors.Is(cause, context.Canceled) {
			reply.status = 499
		}
		if errors.Is(cause, context.DeadlineExceeded) {
			reply.status = 504
		}
	}
	if reply.status < 200 || reply.status > 599 {
		reply.status = 502
	}
	if reply.status == http.StatusOK {
		reply.writer.Header().Set("Content-Type", "application/proto")
		reply.writer.Header().Set("Connect-Protocol-Version", "1")
		reply.writer.WriteHeader(reply.status)
		_, err := reply.writer.Write(reply.body.Bytes())
		return err
	}
	reply.writer.Header().Set("Content-Type", "application/json")
	reply.writer.Header().Set("Connect-Protocol-Version", "1")
	reply.writer.WriteHeader(reply.status)
	return json.NewEncoder(reply.writer).Encode(map[string]string{"code": catalogErrorCode(reply.status), "message": "Native account operation failed"})
}
