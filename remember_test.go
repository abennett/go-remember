package remember

import (
	"net/http"
	"os"
	"testing"
	"time"
)

var urls = []string{
	"https://go.dev",
	"https://blog.golang.org/",
	"https://golang.org/ref/spec",
}

func TestMemory(t *testing.T) {
	memClient, err := AddMemory(http.DefaultClient)
	for _, u := range urls {
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := memClient.Transport.(*Memory); !ok {
			t.Fatal("transport was not successfully replaced")
		}
		start := time.Now()
		_, err = memClient.Get(u)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("first request took %v", time.Now().Sub(start))
		start = time.Now()
		_, err = memClient.Get(u)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("second request took %v", time.Now().Sub(start))
	}
}

func TestMain(m *testing.M) {
	code := m.Run()
	os.Remove(memoryDB)
	os.Exit(code)
}
