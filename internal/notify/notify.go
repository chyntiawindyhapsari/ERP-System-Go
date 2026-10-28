// Package notify mengirim event (mis. "sale.created") ke webhook luar,
// biasanya n8n. Pengiriman berjalan di background supaya kasir tidak menunggu.
package notify

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"time"
)

type Notifier struct {
	url    string
	secret string
	client *http.Client
}

// New mengembalikan Notifier. Jika url kosong, Send tidak melakukan apa-apa.
func New(url, secret string) *Notifier {
	return &Notifier{url: url, secret: secret, client: &http.Client{Timeout: 5 * time.Second}}
}

func (n *Notifier) Send(event string, data any) {
	if n == nil || n.url == "" {
		return
	}
	go func() {
		body, err := json.Marshal(map[string]any{
			"event": event,
			"time":  time.Now().Format(time.RFC3339),
			"data":  data,
		})
		if err != nil {
			log.Printf("notify: gagal encode: %v", err)
			return
		}
		req, err := http.NewRequest(http.MethodPost, n.url, bytes.NewReader(body))
		if err != nil {
			log.Printf("notify: url tidak valid: %v", err)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		if n.secret != "" {
			req.Header.Set("X-Webhook-Secret", n.secret)
		}
		resp, err := n.client.Do(req)
		if err != nil {
			log.Printf("notify: gagal kirim %s: %v", event, err)
			return
		}
		_ = resp.Body.Close()
	}()
}
