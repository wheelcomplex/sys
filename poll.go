package sys

import (
	"runtime"
	"sync/atomic"
	"syscall"
	"unsafe"
)

var (
	epfd  int
	pdmgr pdpool
)

func pollInit() (err error) {
	epfd, err = syscall.EpollCreate1(syscall.EPOLL_CLOEXEC)
	return
}

func pollOpen(f *File, rw uint32, pf PollFunc) error {
	pd := pdmgr.alloc()
	if !atomic.CompareAndSwapUintptr(&f.pd, 0, uintptr(unsafe.Pointer(pd))) {
		pdmgr.free(pd)
		return nil
	}
	var ev syscall.EpollEvent
	e := (syscall.EPOLLET | syscall.EPOLLERR) & 0xffffffff
	if (rw | 1) != 0 {
		e |= syscall.EPOLLIN
	}
	if (rw | 2) != 0 {
		e |= syscall.EPOLLOUT
	}
	pd.f = f
	pd.pf = pf
	ev.Events = uint32(e)
	*(**polldesp)(unsafe.Pointer(&ev.Fd)) = pd
	err := syscall.EpollCtl(epfd, syscall.EPOLL_CTL_ADD, f.sysfd, &ev)
	if err != nil {
		pd.recycle()
		pdmgr.free(pd)
	}
	return err
}

func pollClose(f *File) error {
	var ev syscall.EpollEvent
	pd := *(**polldesp)(unsafe.Pointer(&f.pd))
	pd.recycle()
	pdmgr.free(pd)
	return syscall.EpollCtl(epfd, syscall.EPOLL_CTL_DEL, f.sysfd, &ev)
}

func pollWait(ignoreIntr bool) error {
	nevents := runtime.NumCPU()
	if nevents < 64 {
		nevents = 64
	}
	events := make([]syscall.EpollEvent, nevents)
	for {
		n, err := syscall.EpollWait(epfd, events[:], -1)
		if err != nil {
			if err == syscall.EINTR && ignoreIntr {
				continue
			}
			return err
		}
		for i := 0; i < n; i++ {
			rw := uint32(0)
			if (events[i].Events & (syscall.EPOLLIN | syscall.EPOLLERR | syscall.EPOLLRDHUP)) != 0 {
				rw |= 1
			}
			if (events[i].Events & (syscall.EPOLLOUT | syscall.EPOLLERR | syscall.EPOLLHUP)) != 0 {
				rw |= 2
			}
			pd := *(**polldesp)(unsafe.Pointer(&events[i].Fd))
			pd.pf(pd.f, rw)
		}
	}
}
