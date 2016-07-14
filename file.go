package sys

import (
	"io"
	"sync/atomic"
	"syscall"
)

type File struct {
	sysfd  int
	pd     uintptr
	closed int32
	rw     uint32
	pf     PollFunc
	Data   interface{}
}

func (f *File) Fd() int {
	return f.sysfd
}

func (f *File) PollFlags() uint32 {
	if f.rw != 0 {
		return atomic.SwapUint32(&f.rw, 0)
	}
	return 0
}

func (f *File) SetPollFunc(pf PollFunc) {
	f.pf = pf
	if pf != nil {
		if rw := f.PollFlags(); rw != 0 {
			pf(f, rw)
		}
	}
}

func (f *File) Read(p []byte) (n int, err error) {
	n, err = syscall.Read(f.sysfd, p)
	if err == nil && n == 0 {
		err = io.EOF
	}
	return
}

func (f *File) Write(p []byte) (n int, err error) {
	n, err = syscall.Write(f.sysfd, p)
	if n == 0 && err == nil {
		err = io.EOF
	}
	return
}

func (f *File) Close() error {
	if f.closed != 0 || !atomic.CompareAndSwapInt32(&f.closed, 0, 1) {
		return nil
	}
	pollClose(f)
	f.pf = nil
	return syscall.Close(f.sysfd)
}

func onFilePoll(f *File, rw uint32) {
	pf := f.pf
	if f.rw != 0 {
		rw |= atomic.SwapUint32(&f.rw, 0)
	}
	if pf != nil && f.closed == 0 {
		pf(f, rw)
	} else {
		atomicOr32(&f.rw, rw)
	}
}

func openFile(sysfd int) (*File, error) {
	f := &File{sysfd: sysfd}
	err := syscall.SetNonblock(f.sysfd, true)
	if err != nil {
		return nil, err
	}
	err = pollOpen(f, 3, onFilePoll)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func Socket(af, socktype, proto int) (*File, error) {
	s, err := syscall.Socket(af, socktype, proto)
	if err != nil {
		return nil, err
	}
	f, err := openFile(s)
	if err != nil {
		syscall.Close(s)
		return nil, err
	}
	return f, nil
}

func Accept(l *File) (*File, syscall.Sockaddr, error) {
	c, sa, err := syscall.Accept(l.sysfd)
	if err != nil {
		return nil, nil, err
	}
	cfd, err := openFile(c)
	if err != nil {
		syscall.Close(c)
		return nil, nil, err
	}
	return cfd, sa, nil
}

func Pipe(pfs []*File) (err error) {
	var pipe [2]int
	err = syscall.Pipe(pipe[:])
	if err != nil {
		return
	}
	pfs[0], err = openFile(pipe[0])
	if err != nil {
		return
	}
	pfs[1], err = openFile(pipe[1])
	return
}
