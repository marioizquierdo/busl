package busltee

import (
	"bytes"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dmathieu/safebuffer"
)

func TestNoError(t *testing.T) {
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		body, err := ioutil.ReadAll(r.Body)
		defer r.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if string(body[:len(body)]) != "hello world" {
			t.Fatalf("Expected body to be 'hello world'. Got '%q'", body)
		}
	})
	client := &http.Client{
		Transport: &Transport{
			SleepDuration: time.Millisecond,
		},
	}
	res, err := client.Post(server.URL, "", bytes.NewBuffer([]byte("hello world")))
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 {
		t.Fatalf("was expecting 200 got %d", res.StatusCode)
	}
}

func TestDisconnection(t *testing.T) {
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	var callCount int
	var expectedBody string
	stdin := safebuffer.NewMock()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		body, err := ioutil.ReadAll(r.Body)
		defer r.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		stdin.Write([]byte("hello world\n"))

		if string(body[:len(body)]) != expectedBody {
			t.Fatalf("Unexpected body. Expected %q. Got %q", expectedBody, body)
		}
		expectedBody += "hello world\n"
		callCount += 1

		if callCount < 4 {
			server.CloseClientConnections()
		}
	})
	transport := &Transport{
		MaxRetries:    5,
		SleepDuration: time.Millisecond,
	}
	client := &http.Client{Transport: transport}
	res, err := client.Post(server.URL, "", stdin)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 {
		t.Fatalf("was expecting 200 got %d", res.StatusCode)
	}

	if callCount != 4 {
		t.Fatalf("was expecting 5 retries. Got %d", callCount)
	}
}

func TestHTTPError(t *testing.T) {
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	var callCount int
	var expectedBody string
	stdin := safebuffer.NewMock()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		body, err := ioutil.ReadAll(r.Body)
		defer r.Body.Close()
		if err != nil {
			t.Fatal(err)
		}

		if string(body[:len(body)]) != expectedBody {
			t.Fatalf("Unexpected body. Expected %q. Got %q - Attempt %d", expectedBody, body[:len(body)], callCount)
		}

		if callCount < 9 {
			w.WriteHeader(http.StatusServiceUnavailable)

			stdin.Write([]byte("hello world\n"))
			expectedBody += "hello world\n"
			callCount += 1
		}
	})
	transport := &Transport{
		MaxRetries:    10,
		SleepDuration: time.Millisecond,
	}
	client := &http.Client{Transport: transport}
	res, err := client.Post(server.URL, "", stdin)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != 200 {
		t.Fatalf("was expecting 200 got %d", res.StatusCode)
	}

	if callCount != 9 {
		t.Fatalf("was expecting 9 retries. Got %d", callCount)
	}
}

type slowBuffer struct{}

func (s *slowBuffer) Read(p []byte) (int, error) {
	time.Sleep(time.Second)

	content := []byte("hello world")
	copy(p, content)
	return len(content), io.EOF
}

func TestBodyReaderBlocksClosing(t *testing.T) {
	streamer := &slowBuffer{}
	buffer, err := ioutil.TempFile("", "busltee_buffer")
	if err != nil {
		t.Fatal(err)
	}
	defer buffer.Close()

	reader, err := newBodyReader(streamer, buffer)
	if err != nil {
		t.Fatal(err)
	}
	go reader.Close()

	data, err := ioutil.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello world" {
		t.Fatalf("Expected data to be `hello world`. Got %q", data)
	}
}