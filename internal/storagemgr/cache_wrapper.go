package storagemgr

import (
	"github.com/VictoriaMetrics/fastcache"
)

type CacheWrapper struct {
	cache *fastcache.Cache

	metrics *CacheMetrics

	enableMetric bool
}

type CacheMetrics struct {
	CacheHitCounter  int
	CacheMissCounter int

	CacheSize uint64 // total bytes
}

func NewCacheWrapper(megabytesLimit int, enableMetric bool) *CacheWrapper {
	if megabytesLimit <= 0 {
		megabytesLimit = 128
	}

	return &CacheWrapper{
		cache:        fastcache.New(megabytesLimit * 1024 * 1024),
		metrics:      &CacheMetrics{},
		enableMetric: enableMetric,
	}
}

func (c *CacheWrapper) ResetCounterMetrics() {
	c.metrics.CacheMissCounter = 0
	c.metrics.CacheHitCounter = 0
}

func (c *CacheWrapper) ExportMetrics() *CacheMetrics {
	var s fastcache.Stats
	c.cache.UpdateStats(&s)
	c.metrics.CacheSize = s.BytesSize
	return c.metrics
}

func (c *CacheWrapper) Get(k []byte) ([]byte, bool) {
	res, ok := c.cache.HasGet(nil, k)
	if ok {
		c.metrics.CacheHitCounter++
	} else {
		c.metrics.CacheMissCounter++
	}
	return res, ok
}

func (c *CacheWrapper) Has(k []byte) bool {
	return c.cache.Has(k)
}

func (c *CacheWrapper) Enable() bool {
	return c != nil && c.cache != nil
}

func (c *CacheWrapper) Set(k []byte, v []byte) {
	c.cache.Set(k, v)
}

func (c *CacheWrapper) Del(k []byte) {
	c.cache.Del(k)
}

func (c *CacheWrapper) Reset() {
	c.cache.Reset()
	c.metrics = &CacheMetrics{}
}
