package simple

import (
	"container/list"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

func (c *simpleServer) Api() *chi.Mux {
	r := chi.NewRouter()
	r.Get("/statistics", c.statistics)
	return r
}

func (c *simpleServer) statistics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(200)
	flusher, ok := w.(http.Flusher)
	if !ok {
		w.Write([]byte("Unsupported connection method"))
		return
	}

	var elem *list.Element // maybe nil
	var lastElem *list.Element
	for {
		select {
		case <-r.Context().Done():
			return
		default:
			if lastElem != nil {
				elem = lastElem.Next()
			} else {
				elem = c.backend.Front()
			}

			if elem != nil {
				lastElem = elem
				if _, err := w.Write(append(elem.Value.([]byte), '\n')); err != nil {
					c.logger.Warn("Http write error", zap.Error(err))
					return
				}
				flusher.Flush()
			} else {
				time.Sleep(time.Second)
			}
		}
	}
}
