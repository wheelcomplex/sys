package sys

import (
	"bytes"
	"sync"
	"unsafe"
)

type PollFunc func(*File, uint32)

type polldesp struct {
	f    *File
	pf   PollFunc
	slot uintptr
	idx  int
}

func (pd *polldesp) recycle() {
	pd.f = nil
	pd.pf = nil
}

const slotMax = 64 * 2 // multiple of cache-line size

type pdslot struct {
	marks [slotMax]byte
	pds   [slotMax]polldesp
	// TODO lock-free
	m    sync.Mutex
	used int32
	prev *pdslot
	next *pdslot
}

func slotadd(head **pdslot, slot *pdslot) {
	if *head == nil {
		slot.prev = slot
		slot.next = slot
		*head = slot
	} else {
		s := *head
		slot.next = s.next
		slot.prev = s
		slot.next.prev = slot
		s.next = slot
	}
}

func slotremove(head **pdslot, slot *pdslot) {
	if slot == slot.next {
		*head = nil
	} else {
		slot.next.prev = slot.prev
		slot.prev.next = slot.next
		if *head == slot {
			*head = slot.next
		}
	}
	slot.prev = nil
	slot.next = nil
}

type pdpool struct {
	m       sync.Mutex
	full    *pdslot
	partial *pdslot
}

func (p *pdpool) getslot() *pdslot {
	var slot *pdslot
	p.m.Lock()
	defer p.m.Unlock()
	if p.partial != nil {
		slot = p.partial
		slot.used++
		if slot.used == slotMax {
			slotremove(&p.partial, slot)
			slotadd(&p.full, slot)
		}
	} else {
		slot = new(pdslot)
		slot.used = 1
		slot.next = slot
		slot.prev = slot
		p.partial = slot
	}
	return slot
}

func (p *pdpool) putslot(slot *pdslot) {
	p.m.Lock()
	defer p.m.Unlock()
	if slot.used == slotMax {
		slotremove(&p.full, slot)
		slotadd(&p.partial, slot)
	}
	slot.used--
}

func (p *pdpool) alloc() *polldesp {
	slot := p.getslot()
	slot.m.Lock()
	defer slot.m.Unlock()
	i := bytes.IndexByte(slot.marks[:], 0)
	slot.pds[i].slot = uintptr(unsafe.Pointer(slot))
	slot.pds[i].idx = i
	slot.marks[i] = 1
	return &slot.pds[i]
}

func (p *pdpool) free(pd *polldesp) {
	ok := false
	slot := *(**pdslot)(unsafe.Pointer(&pd.slot))
	slot.m.Lock()
	if slot.marks[pd.idx] == 1 {
		slot.marks[pd.idx] = 0
		ok = true
	}
	slot.m.Unlock()
	if ok {
		p.putslot(slot)
	}
}
