package watcher

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

type Watcher struct {
	watcher *fsnotify.Watcher
	done    chan struct{}
	wg      sync.WaitGroup
}

func New(dir string, onChange func()) (*Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	if err := addRecursive(fw, dir); err != nil {
		fw.Close()
		return nil, err
	}

	w := &Watcher{
		watcher: fw,
		done:    make(chan struct{}),
	}

	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		w.run(onChange)
	}()

	return w, nil
}

func addRecursive(fw *fsnotify.Watcher, root string) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == ".terraform" || base == ".terragrunt-cache" || base == "node_modules" {
				return filepath.SkipDir
			}
			return fw.Add(path)
		}
		return nil
	})
}

func (w *Watcher) run(onChange func()) {
	var debounceTimer *time.Timer
	var mu sync.Mutex

	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			ext := strings.ToLower(filepath.Ext(event.Name))
			if ext != ".tf" && ext != ".tfvars" {
				continue
			}
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) == 0 {
				continue
			}

			mu.Lock()
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			debounceTimer = time.AfterFunc(500*time.Millisecond, onChange)
			mu.Unlock()

		case _, ok := <-w.watcher.Errors:
			if !ok {
				return
			}

		case <-w.done:
			return
		}
	}
}

func (w *Watcher) Close() error {
	close(w.done)
	err := w.watcher.Close()
	w.wg.Wait()
	return err
}
