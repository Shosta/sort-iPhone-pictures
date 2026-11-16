package main

import (
	"container/list"
	"sync"
)

// thumbCache keeps recently used JPEG thumbnails within a byte budget.
type thumbCache struct {
	lock   sync.Mutex
	budget int
	used   int
	order  *list.List // front is the most recently used
	items  map[string]*list.Element
}

type cachedThumb struct {
	path string
	data []byte
	side int // good for requests up to this size
}

func newThumbCache(budget int) *thumbCache {
	return &thumbCache{budget: budget, order: list.New(), items: map[string]*list.Element{}}
}

func (c *thumbCache) get(path string, size int) ([]byte, bool) {
	c.lock.Lock()
	defer c.lock.Unlock()
	element, ok := c.items[path]
	if !ok || element.Value.(*cachedThumb).side < size {
		return nil, false
	}
	c.order.MoveToFront(element)
	return element.Value.(*cachedThumb).data, true
}

// put stores a thumbnail unless a larger one is already cached.
func (c *thumbCache) put(path string, data []byte, side int) {
	c.lock.Lock()
	defer c.lock.Unlock()
	if element, ok := c.items[path]; ok {
		old := element.Value.(*cachedThumb)
		if old.side > side {
			return
		}
		c.used -= len(old.data)
		c.order.Remove(element)
	}
	c.items[path] = c.order.PushFront(&cachedThumb{path, data, side})
	c.used += len(data)
	for c.used > c.budget && c.order.Len() > 1 {
		oldest := c.order.Back()
		c.order.Remove(oldest)
		item := oldest.Value.(*cachedThumb)
		delete(c.items, item.path)
		c.used -= len(item.data)
	}
}
