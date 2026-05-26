package webhooks

import (
	"encoding/json"
	"io"
	"net/http"

	"payment-gateway/internal/dto"
	"payment-gateway/internal/subsystems/digitalruble"
)

type DigitalRubleSOAPHandler struct{}

func NewDigitalRubleSOAPHandler() http.Handler {
	return &DigitalRubleSOAPHandler{}
}

func (h *DigitalRubleSOAPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeDigitalRubleSOAPError(w, http.StatusMethodNotAllowed, dto.ErrorMethodNotAllowed, "use POST")
		return
	}
	defer r.Body.Close()

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 128*1024))
	if err != nil {
		writeDigitalRubleSOAPError(w, http.StatusBadRequest, dto.ErrorDigitalRubleSOAP, err.Error())
		return
	}
	_, responseXML, err := digitalruble.ProcessPaymentCheckSOAP(body)
	if err != nil {
		writeDigitalRubleSOAPError(w, http.StatusBadRequest, dto.ErrorDigitalRubleSOAP, err.Error())
		return
	}

	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(responseXML)
}

func writeDigitalRubleSOAPError(w http.ResponseWriter, status int, code string, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(dto.NewGatewayError(code, message))
}
