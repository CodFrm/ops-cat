// pkg/extension/io_http_test.go
package extension

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

// readerFunc adapts a Read function to io.Reader for use with io.ReadAll.
type readerFunc struct{ fn func([]byte) (int, error) }

func (r *readerFunc) Read(p []byte) (int, error) { return r.fn(p) }

func TestHTTPHandle(t *testing.T) {
	Convey("httpHandle", t, func() {

		Convey("GET request: open, flush, read response, close", func() {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("hello from server"))
			}))
			defer srv.Close()

			h, err := newHTTPHandle(IOOpenParams{
				Method:       "GET",
				URL:          srv.URL + "/test",
				AllowPrivate: true, // httptest server binds to loopback
			}, nil)
			So(err, ShouldBeNil)
			So(h, ShouldNotBeNil)

			meta, err := h.Flush()
			So(err, ShouldBeNil)
			So(meta, ShouldNotBeNil)
			So(meta.Status, ShouldEqual, 200)
			So(meta.ContentType, ShouldEqual, "text/plain")

			all, err := io.ReadAll(&readerFunc{fn: h.Read})
			So(err, ShouldBeNil)
			So(string(all), ShouldEqual, "hello from server")

			So(h.Close(), ShouldBeNil)
		})

		Convey("POST with body: open, write body, flush, read response, close", func() {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte("got:" + string(body)))
			}))
			defer srv.Close()

			h, err := newHTTPHandle(IOOpenParams{
				Method:       "POST",
				URL:          srv.URL + "/submit",
				Headers:      map[string]string{"Content-Type": "application/json"},
				AllowPrivate: true, // httptest server binds to loopback
			}, nil)
			So(err, ShouldBeNil)

			n, err := h.Write([]byte(`{"key":"value"}`))
			So(err, ShouldBeNil)
			So(n, ShouldEqual, 15)

			meta, err := h.Flush()
			So(err, ShouldBeNil)
			So(meta.Status, ShouldEqual, 201)
			So(meta.ContentType, ShouldEqual, "text/plain")

			all, err := io.ReadAll(&readerFunc{fn: h.Read})
			So(err, ShouldBeNil)
			So(string(all), ShouldEqual, `got:{"key":"value"}`)

			So(h.Close(), ShouldBeNil)
		})

		Convey("write after flush returns error", func() {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			h, err := newHTTPHandle(IOOpenParams{
				Method:       "POST",
				URL:          srv.URL,
				AllowPrivate: true, // httptest server binds to loopback
			}, nil)
			So(err, ShouldBeNil)

			meta, err := h.Flush()
			So(err, ShouldBeNil)
			So(meta.Status, ShouldEqual, 200)

			_, err = h.Write([]byte("too late"))
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "not in writing phase")

			So(h.Close(), ShouldBeNil)
		})

		Convey("read before flush returns error", func() {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			h, err := newHTTPHandle(IOOpenParams{
				Method:       "GET",
				URL:          srv.URL,
				AllowPrivate: true, // httptest server binds to loopback
			}, nil)
			So(err, ShouldBeNil)

			_, err = h.Read(make([]byte, 10))
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "not flushed")

			So(h.Close(), ShouldBeNil)
		})

		Convey("close cancels in-flight request", func() {
			started := make(chan struct{})
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				close(started)
				// Block forever; the client should cancel
				<-r.Context().Done()
			}))
			defer srv.Close()

			h, err := newHTTPHandle(IOOpenParams{
				Method:       "GET",
				URL:          srv.URL,
				AllowPrivate: true, // httptest server binds to loopback
			}, nil)
			So(err, ShouldBeNil)

			// Flush in a goroutine since the server blocks
			errCh := make(chan error, 1)
			go func() {
				_, err := h.Flush()
				errCh <- err
			}()

			// Wait for server to receive request, then close
			<-started
			So(h.Close(), ShouldBeNil)

			// Flush should have returned an error
			err = <-errCh
			So(err, ShouldNotBeNil)
		})

		Convey("custom headers are sent", func() {
			var receivedHeader string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedHeader = r.Header.Get("X-Custom")
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			h, err := newHTTPHandle(IOOpenParams{
				Method:       "GET",
				URL:          srv.URL,
				Headers:      map[string]string{"X-Custom": "test-value"},
				AllowPrivate: true, // httptest server binds to loopback
			}, nil)
			So(err, ShouldBeNil)

			_, err = h.Flush()
			So(err, ShouldBeNil)
			So(receivedHeader, ShouldEqual, "test-value")
			So(h.Close(), ShouldBeNil)
		})

		Convey("custom dial function is used", func() {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("tunneled"))
			}))
			defer srv.Close()

			dialCalled := false
			customDial := func(network, addr string) (net.Conn, error) {
				dialCalled = true
				// Delegate to real dialer so the request actually succeeds
				return net.Dial(network, addr)
			}

			h, err := newHTTPHandle(IOOpenParams{
				Method:       "GET",
				URL:          srv.URL,
				AllowPrivate: true, // httptest server binds to loopback
			}, customDial)
			So(err, ShouldBeNil)

			meta, err := h.Flush()
			So(err, ShouldBeNil)
			So(meta.Status, ShouldEqual, 200)
			So(dialCalled, ShouldBeTrue)

			all, err := io.ReadAll(&readerFunc{fn: h.Read})
			So(err, ShouldBeNil)
			So(string(all), ShouldEqual, "tunneled")

			So(h.Close(), ShouldBeNil)
		})
	})
}

func TestIOHandleManagerHTTP(t *testing.T) {
	Convey("IOHandleManager HTTP", t, func() {
		mgr := NewIOHandleManager()
		defer mgr.CloseAll()

		Convey("OpenHTTP and Flush round-trip", func() {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("manager test"))
			}))
			defer srv.Close()

			id, meta, err := mgr.OpenHTTP(IOOpenParams{
				Method:       "GET",
				URL:          srv.URL,
				AllowPrivate: true, // httptest server binds to loopback
			}, nil)
			So(err, ShouldBeNil)
			So(id, ShouldBeGreaterThan, 0)
			So(meta.Status, ShouldEqual, 0) // no status yet before flush

			flushed, err := mgr.Flush(id)
			So(err, ShouldBeNil)
			So(flushed.Status, ShouldEqual, 200)

			all, err := io.ReadAll(&readerFunc{fn: func(p []byte) (int, error) {
				return mgr.Read(id, p)
			}})
			So(err, ShouldBeNil)
			So(string(all), ShouldEqual, "manager test")

			So(mgr.Close(id), ShouldBeNil)
		})

		Convey("Flush on non-HTTP handle returns error", func() {
			// Register a plain handle (non-HTTP)
			r := strings.NewReader("hello")
			id, regErr := mgr.Register(r, nil, io.NopCloser(r), IOMeta{})
			So(regErr, ShouldBeNil)

			_, err := mgr.Flush(id)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "not an HTTP handle")
		})

		Convey("Flush on unknown handle returns error", func() {
			_, err := mgr.Flush(99999)
			So(err, ShouldNotBeNil)
		})
	})
}
