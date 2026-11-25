package portal

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/fasthttp/websocket"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tokpath/tokstudio/backend/internal/catalog"
	"github.com/tokpath/tokstudio/backend/internal/ledger"
	"github.com/tokpath/tokstudio/backend/internal/modelruntime"
	"github.com/tokpath/tokstudio/backend/internal/usage"
)

const responsesSocketLimit = 16 << 20

type responsesSocketTurn struct {
	input        modelruntime.Request
	businessKey  string
	eventID      string
	streamID     string
	previousID   string
	generate     bool
	hasClientKey bool
}

func decodeResponsesSocketTurn(raw []byte) (responsesSocketTurn, error) {
	turn := responsesSocketTurn{generate: true}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || string(fields["type"]) != `"response.create"` {
		return turn, modelruntime.ErrUnsupported
	}
	var idempotency string
	for name, target := range map[string]*string{
		"event_id": &turn.eventID, "stream_id": &turn.streamID, "previous_response_id": &turn.previousID, "idempotency_key": &idempotency,
	} {
		if value := fields[name]; value != nil && (json.Unmarshal(value, target) != nil || len(*target) > 120) {
			return turn, modelruntime.ErrInvalidRequest
		}
	}
	if value := fields["generate"]; value != nil && (string(value) == "null" || json.Unmarshal(value, &turn.generate) != nil) {
		return turn, modelruntime.ErrInvalidRequest
	}
	if value := fields["stream"]; value != nil && string(value) != "true" {
		return turn, modelruntime.ErrInvalidRequest
	}
	for _, key := range []string{"type", "event_id", "stream_id", "idempotency_key", "stream"} {
		delete(fields, key)
	}
	// The fingerprint covers the client payload before expanding the history.
	// Retrying the same turn can therefore find its bill after reconnecting.
	fingerprint, _ := json.Marshal(fields)
	delete(fields, "previous_response_id")
	delete(fields, "generate")
	fields["stream"] = json.RawMessage("true")
	if fields["store"] == nil {
		fields["store"] = json.RawMessage("false")
	}
	body, _ := json.Marshal(fields)
	var err error
	turn.input, err = modelruntime.DecodeProtocol("", modelruntime.Responses, body)
	if err != nil {
		return turn, err
	}
	turn.input.WebSocketInput = fingerprint
	if idempotency == "" {
		idempotency = turn.eventID
	}
	turn.hasClientKey = idempotency != ""
	if idempotency == "" {
		idempotency = uuid.NewString()
	}
	turn.businessKey = "responses-ws:" + idempotency
	return turn, nil
}

type responsesSocketContext struct {
	streamID string
	id       string
	model    string
	items    []json.RawMessage
	bytes    int
}

type responsesSocket struct {
	conn      *websocket.Conn
	history   []responsesSocketContext
	totalSize int
}

func (s *responsesSocket) previous(turn responsesSocketTurn) ([]json.RawMessage, bool) {
	if turn.previousID == "" {
		return nil, true
	}
	for _, prior := range s.history {
		if prior.id == turn.previousID && prior.model == turn.input.PublicModel {
			return prior.items, true
		}
	}
	return nil, false
}

func (s *responsesSocket) remember(turn responsesSocketTurn, completion []byte) {
	var event struct {
		Response struct {
			ID     string            `json:"id"`
			Output []json.RawMessage `json:"output"`
		} `json:"response"`
	}
	prior := responsesSocketContext{streamID: turn.streamID, model: turn.input.PublicModel}
	if json.Unmarshal(completion, &event) != nil || event.Response.ID == "" || json.Unmarshal(turn.input.Input, &prior.items) != nil {
		return
	}
	prior.id = event.Response.ID
	prior.items = append(prior.items, event.Response.Output...)
	for _, item := range prior.items {
		prior.bytes += len(item) + 1
	}
	if prior.bytes > responsesSocketLimit {
		return
	}
	for i, old := range s.history {
		if old.streamID == prior.streamID {
			s.totalSize -= old.bytes
			s.history = slices.Delete(s.history, i, i+1)
			break
		}
	}
	for len(s.history) >= 8 || s.totalSize+prior.bytes > responsesSocketLimit {
		s.totalSize -= s.history[0].bytes
		s.history = slices.Delete(s.history, 0, 1)
	}
	s.history, s.totalSize = append(s.history, prior), s.totalSize+prior.bytes
}

func (s *responsesSocket) write(event map[string]json.RawMessage, turn responsesSocketTurn, requestID string) error {
	for key, value := range map[string]string{
		"stream_id": turn.streamID, "client_event_id": turn.eventID, "request_id": requestID,
	} {
		if value != "" {
			event[key], _ = json.Marshal(value)
		}
	}
	if err := s.conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	return s.conn.WriteJSON(event)
}

func (s *responsesSocket) fail(turn responsesSocketTurn, requestID string, status int, code, message string) error {
	kind := "invalid_request_error"
	if status >= 500 {
		kind = "server_error"
	} else if status == http.StatusTooManyRequests {
		kind = "rate_limit_error"
	}
	detail, _ := json.Marshal(gin.H{"type": kind, "code": code, "message": message})
	rawStatus, _ := json.Marshal(status)
	return s.write(map[string]json.RawMessage{"type": json.RawMessage(`"error"`),
		"status": rawStatus, "error": detail}, turn, requestID)
}

func (s *responsesSocket) requestError(turn responsesSocketTurn, id string, err error) error {
	switch {
	case errors.Is(err, usage.ErrKey):
		return s.fail(turn, id, 401, "invalid_api_key", "API key is no longer valid")
	case errors.Is(err, usage.ErrRate), errors.Is(err, usage.ErrBudget), errors.Is(err, usage.ErrConcurrency),
		errors.Is(err, ledger.ErrInsufficient):
		return s.fail(turn, id, 429, "request_limit", "Request exceeds an available budget or capacity limit")
	case errors.Is(err, usage.ErrConflict):
		return s.fail(turn, id, 409, "idempotency_conflict", "This request key already belongs to another payload")
	case errors.Is(err, usage.ErrInProgress):
		return s.fail(turn, id, 409, "in_progress", "The original request is still in progress")
	case errors.Is(err, usage.ErrResultUnavailable):
		return s.fail(turn, id, 410, "result_unavailable", "The original response is no longer available")
	case errors.Is(err, modelruntime.ErrUnsupported), errors.Is(err, modelruntime.ErrInvalidRequest), errors.Is(err, catalog.ErrModelNotVisible):
		return s.fail(turn, id, 400, "invalid_request", "The request or model is unavailable")
	}
	return s.fail(turn, id, 503, "request_unavailable", "The request could not be completed")
}

func (a *Application) responsesWebSocket(c *gin.Context) {
	key, ok := a.apiKey(c)
	if !ok {
		return
	}
	if !websocket.IsWebSocketUpgrade(c.Request) {
		c.Header("Upgrade", "websocket")
		fail(c, http.StatusUpgradeRequired, "upgrade_required", "请使用 WebSocket 连接", false)
		return
	}
	upgrader := websocket.Upgrader{HandshakeTimeout: 5 * time.Second}
	c.Status(http.StatusSwitchingProtocols)
	conn, err := upgrader.Upgrade(c.Writer, c.Request, c.Writer.Header())
	if err != nil {
		return
	}
	defer conn.Close()
	conn.SetReadLimit(responsesSocketLimit)
	if conn.SetReadDeadline(time.Now().Add(time.Hour)) != nil {
		return
	}
	socket := responsesSocket{conn: conn}
	for {
		kind, body, err := conn.ReadMessage()
		if err != nil {
			return
		}
		turn, err := decodeResponsesSocketTurn(body)
		if kind != websocket.TextMessage || err != nil {
			if socket.fail(turn, "", 400, "invalid_request", "Send a JSON response.create event") != nil {
				return
			}
			continue
		}
		// An accepted turn can finish and settle after its socket disconnects.
		ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), a.Usage.UsageTimeout+30*time.Second)
		err = a.executeSocketTurn(ctx, &socket, key, turn)
		cancel()
		if err != nil {
			return
		}
	}
}

func (a *Application) executeSocketTurn(ctx context.Context, socket *responsesSocket, key usage.Key, turn responsesSocketTurn) error {
	previous, present := socket.previous(turn)
	if turn.generate && turn.hasClientKey {
		replay, found, err := a.Usage.ReplayResponseTurn(ctx, key, turn.businessKey, turn.input)
		if err != nil {
			return socket.requestError(turn, replay.Request.ID, err)
		}
		if found {
			if present && turn.input.ResponsesContext(previous) == nil {
				socket.remember(turn, replay.Result.Output)
			}
			return socket.completion(turn, replay.Request.ID, replay.Result.Output)
		}
	}
	if !present {
		return socket.fail(turn, "", 400, "previous_response_not_found", "Send full input context to start on this connection")
	}
	if err := turn.input.ResponsesContext(previous); err != nil {
		return socket.requestError(turn, "", err)
	}
	if !turn.generate {
		return socket.warmup(turn)
	}
	if string(turn.input.Input) == "[]" {
		return socket.requestError(turn, "", modelruntime.ErrInvalidRequest)
	}
	disconnected := false
	execution, err := a.Usage.Execute(ctx, key, turn.businessKey, turn.input, func(_ context.Context, chunk modelruntime.Event) error {
		var event map[string]json.RawMessage
		if json.Unmarshal(chunk.Data, &event) != nil || event == nil {
			return modelruntime.ErrInvalidRequest
		}
		switch string(event["type"]) {
		case `"response.completed"`, `"response.incomplete"`:
			// Send the terminal event after the request has settled.
		default:
			if !disconnected {
				disconnected = socket.write(event, turn, chunk.RequestID) != nil
			}
		}
		return nil
	})
	if disconnected {
		return websocket.ErrCloseSent
	}
	if err != nil {
		return socket.requestError(turn, execution.Request.ID, err)
	}
	if execution.Request.Status != "completed" || len(execution.Result.Output) == 0 {
		return socket.requestError(turn, execution.Request.ID, usage.ErrInProgress)
	}
	socket.remember(turn, execution.Result.Output)
	return socket.completion(turn, execution.Request.ID, execution.Result.Output)
}

func (s *responsesSocket) completion(turn responsesSocketTurn, id string, body []byte) error {
	var completion map[string]json.RawMessage
	if json.Unmarshal(body, &completion) != nil || completion == nil {
		return s.requestError(turn, id, usage.ErrResultUnavailable)
	}
	return s.write(completion, turn, id)
}

func (s *responsesSocket) warmup(turn responsesSocketTurn) error {
	response := gin.H{"id": "resp_" + uuid.NewString(), "object": "response", "status": "in_progress",
		"model": turn.input.PublicModel, "created_at": time.Now().Unix(), "output": []string{}}
	raw, _ := json.Marshal(response)
	if err := s.write(map[string]json.RawMessage{"type": json.RawMessage(`"response.created"`), "response": raw}, turn, ""); err != nil {
		return err
	}
	response["status"] = "completed"
	response["usage"] = gin.H{"input_tokens": 0, "output_tokens": 0, "total_tokens": 0}
	raw, _ = json.Marshal(response)
	event, _ := json.Marshal(map[string]json.RawMessage{"type": json.RawMessage(`"response.completed"`), "response": raw})
	s.remember(turn, event)
	return s.completion(turn, "", event)
}
