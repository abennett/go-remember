package remember

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"

	bolt "go.etcd.io/bbolt"
)

var (
	memoryBucket = []byte("memory")
	memoryDB     = "memory.bdb"
)

type Memory struct {
	bdb       *bolt.DB
	defaultRT http.RoundTripper
}

type RememberedResponse struct {
	Status           string
	StatusCode       int
	Proto            string
	ProtoMajor       int
	ProtoMinor       int
	Header           http.Header
	Body             []byte
	TransferEncoding []string
	Trailer          http.Header
	Request          *http.Request
}

func New(rt http.RoundTripper) (*Memory, error) {
	db, err := bolt.Open(memoryDB, 0600, nil)
	if err != nil {
		return nil, fmt.Errorf("unable to open boltdb file: %w", err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(memoryBucket)
		if err != nil {
			return fmt.Errorf("failed updating bucket: %w", err)
		}
		return nil
	})
	if rt == nil {
		rt = http.DefaultTransport
	}
	return &Memory{
		bdb:       db,
		defaultRT: rt,
	}, nil
}

func AddMemory(client *http.Client) (*http.Client, error) {
	transport, err := New(client.Transport)
	if err != nil {
		return client, err
	}
	client.Transport = transport
	return client, nil
}

func (m *Memory) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := m.fetchResponse(req.URL)
	if err != nil {
		return nil, fmt.Errorf("fetch error: %w", err)
	}
	if resp != nil {
		log.Println("fetched from memory")
		return resp, nil
	}
	log.Println("using default transport")
	resp, err = m.defaultRT.RoundTrip(req)
	if err != nil {
		return nil, fmt.Errorf("default round tripper error: %w", err)
	}
	log.Println("storing response")
	err = m.storeResponse(resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (m *Memory) storeResponse(resp *http.Response) error {
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("unable to read response body: %w", err)
	}
	resp.Body.Close()
	resp.Body = ioutil.NopCloser(bytes.NewReader(body))
	rr := &RememberedResponse{
		Status:           resp.Status,
		StatusCode:       resp.StatusCode,
		Proto:            resp.Proto,
		ProtoMajor:       resp.ProtoMajor,
		ProtoMinor:       resp.ProtoMinor,
		Header:           resp.Header,
		Body:             body,
		TransferEncoding: resp.TransferEncoding,
		Trailer:          resp.Trailer,
		Request:          resp.Request,
	}
	err = m.bdb.Update(func(tx *bolt.Tx) error {
		var b bytes.Buffer
		encoder := gob.NewEncoder(&b)
		err = encoder.Encode(rr)
		if err != nil {
			return fmt.Errorf("unable to encode RememberedResponse: %w", err)
		}
		bucket := tx.Bucket(memoryBucket)
		ub, err := resp.Request.URL.MarshalBinary()
		if err != nil {
			return err
		}
		err = bucket.Put(ub, b.Bytes())
		if err != nil {
			return fmt.Errorf("failed to put in bucket: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("unable to store response: %w", err)
	}
	return nil
}

func (m *Memory) fetchResponse(u *url.URL) (*http.Response, error) {
	ub, err := u.MarshalBinary()
	if err != nil {
		return nil, err
	}
	var found bool
	rr := &RememberedResponse{}
	err = m.bdb.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(memoryBucket)
		v := b.Get(ub)
		if v == nil {
			log.Printf("%v not found", u)
			return nil
		}
		found = true
		reader := bytes.NewReader(v)
		decoder := gob.NewDecoder(reader)
		return decoder.Decode(rr)
	})
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	return &http.Response{
		Status:           rr.Status,
		StatusCode:       rr.StatusCode,
		Proto:            rr.Proto,
		ProtoMajor:       rr.ProtoMajor,
		ProtoMinor:       rr.ProtoMinor,
		Header:           rr.Header,
		Body:             ioutil.NopCloser(bytes.NewReader(rr.Body)),
		ContentLength:    int64(len(rr.Body)),
		TransferEncoding: rr.TransferEncoding,
		Uncompressed:     true,
		Trailer:          rr.Trailer,
		Request:          rr.Request,
	}, nil
}
