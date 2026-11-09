// Package finalmask — клиентские маски соединения в формате Xray finalmask.
// Маска стоит между сокетом и TLS/REALITY: режет то, что пишет слой выше.
package finalmask

import (
	"net"
	"time"

	"github.com/sagernet/sing-box/common/xray/crypto"
	"github.com/sagernet/sing-box/option"
)

type fragmentConfig struct {
	packetsFrom, packetsTo int64
	lengthsMin, lengthsMax []int64
	delaysMin, delaysMax   []int64
	maxSplitMin            int64
	maxSplitMax            int64
}

func newFragmentConfig(o option.FragmentMaskOptions) (*fragmentConfig, error) {
	from, to, err := o.PacketsRange()
	if err != nil {
		return nil, err
	}
	c := &fragmentConfig{
		packetsFrom: from,
		packetsTo:   to,
		maxSplitMin: int64(o.MaxSplit.From),
		maxSplitMax: int64(o.MaxSplit.To),
	}
	for _, r := range o.LengthRanges() {
		c.lengthsMin = append(c.lengthsMin, int64(r.From))
		c.lengthsMax = append(c.lengthsMax, int64(r.To))
	}
	for _, r := range o.DelayRanges() {
		c.delaysMin = append(c.delaysMin, int64(r.From))
		c.delaysMax = append(c.delaysMax, int64(r.To))
	}
	return c, nil
}

// fragmentConn — порт xray-core transport/internet/finalmask/fragment/conn.go
// (v26.10.31). Upstream() намеренно не реализован: копирование sing через
// развёртку писало бы мимо маски.
type fragmentConn struct {
	net.Conn
	config *fragmentConfig
	count  uint64
}

func (c *fragmentConfig) segment(list []int64, i int) int64 {
	if i >= len(list) {
		i = len(list) - 1
	}
	return list[i]
}

func (c *fragmentConfig) length(i int) int64 {
	return crypto.RandBetween(c.segment(c.lengthsMin, i), c.segment(c.lengthsMax, i))
}

func (c *fragmentConfig) sleep(i int) {
	if max := c.segment(c.delaysMax, i); max > 0 {
		time.Sleep(time.Duration(crypto.RandBetween(c.segment(c.delaysMin, i), max)) * time.Millisecond)
	}
}

// При единственной нулевой задержке записи ClientHello уходят одним Write.
func (c *fragmentConfig) mergeHello() bool {
	return len(c.delaysMax) == 1 && c.delaysMax[0] == 0
}

func (c *fragmentConn) Write(p []byte) (int, error) {
	c.count++
	if c.config.packetsFrom == 0 && c.config.packetsTo == 1 {
		return c.writeTLSHello(p)
	}
	if c.config.packetsFrom != 0 && (c.count < uint64(c.config.packetsFrom) || c.count > uint64(c.config.packetsTo)) {
		return c.Conn.Write(p)
	}
	maxSplit := crypto.RandBetween(c.config.maxSplitMin, c.config.maxSplitMax)
	var splitNum int64
	for from := 0; ; {
		to := from + int(c.config.length(int(splitNum)))
		if to > len(p) || (maxSplit > 0 && splitNum+1 >= maxSplit) {
			to = len(p)
		}
		n, err := c.Conn.Write(p[from:to])
		from += n
		if err != nil {
			return from, err
		}
		c.config.sleep(int(splitNum))
		splitNum++
		if from >= len(p) {
			return from, nil
		}
	}
}

// writeTLSHello режет первую запись handshake (ClientHello) на TLS-записи
// длиной length, каждой — свой 5-байтный заголовок.
func (c *fragmentConn) writeTLSHello(p []byte) (int, error) {
	if c.count != 1 || len(p) <= 5 || p[0] != 22 {
		return c.Conn.Write(p)
	}
	recordLen := 5 + (int(p[3])<<8 | int(p[4]))
	if len(p) < recordLen {
		return c.Conn.Write(p)
	}
	data := p[5:recordLen]
	merge := c.config.mergeHello()
	maxSplit := crypto.RandBetween(c.config.maxSplitMin, c.config.maxSplitMax)
	var merged []byte
	for from, splitNum := 0, int64(0); from < len(data); splitNum++ {
		to := from + int(c.config.length(int(splitNum)))
		if to > len(data) || (maxSplit > 0 && splitNum+1 >= maxSplit) {
			to = len(data)
		}
		l := to - from
		record := make([]byte, 0, 5+l)
		record = append(record, p[:3]...)
		record = append(record, byte(l>>8), byte(l))
		record = append(record, data[from:to]...)
		from = to
		if merge {
			merged = append(merged, record...)
			continue
		}
		if _, err := c.Conn.Write(record); err != nil {
			return 0, err
		}
		c.config.sleep(int(splitNum))
	}
	if len(merged) > 0 {
		if _, err := c.Conn.Write(merged); err != nil {
			return 0, err
		}
	}
	if len(p) > recordLen {
		n, err := c.Conn.Write(p[recordLen:])
		if err != nil {
			return recordLen + n, err
		}
	}
	return len(p), nil
}
