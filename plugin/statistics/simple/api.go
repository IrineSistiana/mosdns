package simple

import (
	"container/list"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

func (c *UiServer) Api() *chi.Mux {
	r := chi.NewRouter()
	r.Get("/statistics", c.statistics)
	return r
}

func (c *UiServer) statistics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(200)
	if _, ok := w.(http.Flusher); !ok {
		w.Write([]byte("Unsupported connection method"))
		return
	}
	requestOver := make(chan struct{})

	go func() {
		select {
		case <-requestOver:
		case <-r.Context().Done():
		}

	}()

	var elem, lastElem *list.Element
	for {
		select {
		case <-r.Context().Done():
			return
		default:
			if elem != nil {
				lastElem = elem
				if _, err := w.Write(append(elem.Value.([]byte), '\n')); err != nil {
					close(requestOver)
					return
				}
				w.(http.Flusher).Flush()
			} else {
				time.Sleep(time.Second)
			}

			if lastElem != nil {
				elem = lastElem.Next()
			} else {
				elem = c.backend.Front()
			}
		}
	}
}
