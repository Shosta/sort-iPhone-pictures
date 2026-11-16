// Command photosort-sidecar serves the sorting engine to the Electron app.
//
// Protocol: one JSON request per line on stdin, JSON events (one per line) on
// stdout. Every event carries the "id" of its request. Requests:
//
//	{"id":"1","op":"plan","sources":[...],"destination":"...","byLocation":true,"landmarks":true,"dedupe":true}
//	{"id":"2","op":"copy","planId":"1"}
//	{"id":"3","op":"thumbnail","path":"...","size":160}
//	{"id":"4","op":"cancel","target":"2"}
//
// Events: plan, progress, thumbnail, done and error.
package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"sort-iphone-pictures/internal/thumbs"
	"sort-iphone-pictures/sorter"
)

type request struct {
	ID          string   `json:"id"`
	Op          string   `json:"op"`
	Sources     []string `json:"sources"`
	Destination string   `json:"destination"`
	ByLocation  bool     `json:"byLocation"`
	Landmarks   bool     `json:"landmarks"`
	Dedupe      bool     `json:"dedupe"`
	Similar     bool     `json:"similar"`
	PlanID      string   `json:"planId"`
	Path        string   `json:"path"`
	Size        int      `json:"size"`
	Target      string   `json:"target"`
}

type event map[string]any

type server struct {
	out     io.Writer
	outLock sync.Mutex
	lock    sync.Mutex
	plans   map[string]sorter.Plan
	cancels map[string]context.CancelFunc
	thumbs  chan struct{}
	wg      sync.WaitGroup
}

func main() {
	srv := newServer(os.Stdout)
	if err := srv.serve(context.Background(), os.Stdin); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func newServer(out io.Writer) *server {
	return &server{
		out:     out,
		plans:   map[string]sorter.Plan{},
		cancels: map[string]context.CancelFunc{},
		thumbs:  make(chan struct{}, 4),
	}
}

func (s *server) serve(ctx context.Context, in io.Reader) error {
	ctx, stop := context.WithCancel(ctx)
	defer stop()
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 1<<20), 64<<20)
	for scanner.Scan() {
		var req request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			s.send(event{"id": "", "type": "error", "message": "invalid request: " + err.Error()})
			continue
		}
		s.handle(ctx, req)
	}
	stop()
	s.wg.Wait()
	return scanner.Err()
}

func (s *server) send(e event) {
	s.outLock.Lock()
	defer s.outLock.Unlock()
	data, _ := json.Marshal(e)
	s.out.Write(append(data, '\n'))
}

func (s *server) handle(parent context.Context, req request) {
	if req.Op == "cancel" {
		s.lock.Lock()
		cancel := s.cancels[req.Target]
		s.lock.Unlock()
		if cancel != nil {
			cancel()
		}
		return
	}
	ctx, cancel := context.WithCancel(parent)
	s.lock.Lock()
	s.cancels[req.ID] = cancel
	s.lock.Unlock()
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer func() {
			cancel()
			s.lock.Lock()
			delete(s.cancels, req.ID)
			s.lock.Unlock()
		}()
		if err := s.run(ctx, req); err != nil {
			s.send(event{"id": req.ID, "type": "error", "message": err.Error(), "cancelled": errors.Is(err, context.Canceled)})
		}
	}()
}

func (s *server) run(ctx context.Context, req request) error {
	switch req.Op {
	case "plan":
		plan, err := (sorter.Engine{ByLocation: req.ByLocation, Landmarks: req.Landmarks, SkipDuplicates: req.Dedupe, GroupSimilar: req.Similar}).Plan(ctx, req.Sources, req.Destination)
		if err != nil {
			return err
		}
		s.lock.Lock()
		s.plans[req.ID] = plan
		s.lock.Unlock()
		s.send(event{"id": req.ID, "type": "plan", "plan": plan})
	case "copy":
		s.lock.Lock()
		plan, ok := s.plans[req.PlanID]
		s.lock.Unlock()
		if !ok {
			return errors.New("unknown plan")
		}
		total := len(plan.Entries)
		report, err := sorter.CopyWithProgress(ctx, plan, func(entry sorter.Entry) {
			s.send(event{"id": req.ID, "type": "progress", "entry": entry, "total": total})
		})
		if err == nil {
			s.send(event{"id": req.ID, "type": "done", "copied": report.Copied, "total": total})
		}
		return err
	case "thumbnail":
		select {
		case s.thumbs <- struct{}{}:
			defer func() { <-s.thumbs }()
		case <-ctx.Done():
			return ctx.Err()
		}
		size := req.Size
		if size < 16 || size > 1024 {
			size = 160
		}
		data, err := thumbs.JPEG(req.Path, size)
		if err != nil {
			return err
		}
		s.send(event{"id": req.ID, "type": "thumbnail", "path": req.Path,
			"dataUrl": "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(data)})
	default:
		return fmt.Errorf("unknown op %q", req.Op)
	}
	return nil
}
