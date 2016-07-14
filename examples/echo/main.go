package main

import (
	"io"
	"log"
	"os"
	"syscall"

	"github.com/vizee/sys"
)

const (
	SPLICE_F_MOVE     = 1
	SPLICE_F_NONBLOCK = 2

	EV_R = 1
	EV_W = 2
)

type iocontext struct {
	conn *sys.File
	crw  uint32
	pipe [2]*sys.File
	prw  [2]uint32
	bufn int64
}

var (
	pipeSize int
	chConn   chan *sys.File
)

func echo(ctx *iocontext) error {
	hasRead := false
	for ctx.crw&EV_R != 0 && ctx.prw[1]&EV_W != 0 && pipeSize > int(ctx.bufn) {
		n, err := syscall.Splice(ctx.conn.Fd(), nil, ctx.pipe[1].Fd(), nil, pipeSize-int(ctx.bufn), SPLICE_F_MOVE|SPLICE_F_NONBLOCK)
		if err == nil {
			if n == 0 {
				return io.EOF
			} else {
				hasRead = true
				ctx.bufn += n
			}
		} else {
			if err != syscall.EAGAIN {
				return err
			}
			if !hasRead {
				// 如果 conn 为可读, 但 splice 直接返回 EAGAIN
				// 即使 pipe 没有写到 65536 字节, 依然有可能是 pipe 写满 (page 耗尽)
				ctx.prw[1] = 0
			} else {
				ctx.crw &= EV_W
			}
		}
	}
	for ctx.crw&EV_W != 0 && ctx.prw[0]&EV_R != 0 && ctx.bufn > 0 {
		n, err := syscall.Splice(ctx.pipe[0].Fd(), nil, ctx.conn.Fd(), nil, int(ctx.bufn), SPLICE_F_MOVE|SPLICE_F_NONBLOCK)
		if err == nil {
			if n == 0 {
				return io.EOF
			} else {
				ctx.bufn -= n
			}
		} else {
			if err != syscall.EAGAIN {
				return err
			}
			ctx.crw &= EV_R
		}
	}
	return nil
}

func pollConn() {
	for c := range chConn {
		ctx := c.Data.(*iocontext)
		err := echo(ctx)
		if err != nil {
			log.Println("echo failed:", err)
			ctx.pipe[0].Close()
			ctx.pipe[1].Close()
			ctx.conn.Close()
		}
	}
}

func onListen(f *sys.File, rw uint32) {
	if rw&1 == 0 {
		return
	}
	for {
		conn, sa, err := sys.Accept(f)
		if err != nil {
			if err == syscall.EAGAIN {
				break
			}
			panic(err)
		}
		log.Println("client :", sys.ToNetAddr(sa))
		ctx := new(iocontext)
		ctx.conn = conn
		conn.Data = ctx
		err = sys.Pipe(ctx.pipe[:])
		if err != nil {
			panic(err)
		}
		if pipeSize <= 0 {
			pipeSize, err = sys.Fcntl(ctx.pipe[0].Fd(), syscall.F_GETPIPE_SZ, 0)
			if err != nil {
				panic(err)
			}
		}
		ctx.pipe[0].Data = ctx
		ctx.pipe[0].SetPollFunc(func(f *sys.File, rw uint32) {
			ctx := f.Data.(*iocontext)
			ctx.prw[0] |= rw
			chConn <- ctx.conn
		})
		ctx.pipe[1].Data = ctx
		ctx.pipe[1].SetPollFunc(func(f *sys.File, rw uint32) {
			ctx := f.Data.(*iocontext)
			ctx.prw[1] |= rw
			chConn <- ctx.conn
		})
		conn.SetPollFunc(func(f *sys.File, rw uint32) {
			f.Data.(*iocontext).crw |= rw
			chConn <- f
		})
	}
}

func listen(laddr string) error {
	af, sa, err := sys.ResolveSockaddr(laddr)
	if err != nil {
		return err
	}
	lfd, err := sys.Socket(af, syscall.SOCK_STREAM, 0)
	if err != nil {
		return err
	}
	lfd.SetPollFunc(onListen)
	ok := false
	defer func() {
		if !ok {
			lfd.Close()
		}
	}()
	err = syscall.Bind(lfd.Fd(), sa)
	if err != nil {
		return err
	}
	err = syscall.Listen(lfd.Fd(), syscall.SOMAXCONN)
	if err != nil {
		return err
	}
	ok = true
	return nil
}

func main() {
	if len(os.Args) != 2 {
		println("usage: echo <addr>")
		return
	}
	err := sys.Init()
	if err != nil {
		panic(err)
	}
	log.Println("listen", os.Args[1])
	err = listen(os.Args[1])
	if err != nil {
		panic(err)
	}
	chConn = make(chan *sys.File, 4)
	go pollConn()
	err = sys.PollWait()
	if err != nil {
		panic(err)
	}
}
