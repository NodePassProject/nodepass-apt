// Package conn 对 net.Conn 的扩展，包括超时读取、限速统计和双向数据交换等功能
package conn

import (
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// 定义错误
var (
	ErrNotTCPConn = errors.New("not a TCP connection")
	ErrNotUDPConn = errors.New("not a UDP connection")
)

// RateLimiter 全局令牌桶读写限速器
type RateLimiter struct {
	readRate, writeRate     int64      // 每秒读取/写入字节数
	readTokens, writeTokens int64      // 当前令牌数
	lastUpdate              int64      // 上次更新时间
	condition               *sync.Cond // 条件变量
}

// NewRateLimiter 创建新的全局令牌桶读写限速器
func NewRateLimiter(readBytesPerSecond, writeBytesPerSecond int64) *RateLimiter {
	if readBytesPerSecond <= 0 && writeBytesPerSecond <= 0 {
		return nil
	}

	// 如果某个速率为0，设置为无限制
	if readBytesPerSecond <= 0 {
		readBytesPerSecond = 1 << 40 // 1TB/s
	}
	if writeBytesPerSecond <= 0 {
		writeBytesPerSecond = 1 << 40 // 1TB/s
	}

	rl := &RateLimiter{
		readRate:  readBytesPerSecond,
		writeRate: writeBytesPerSecond,
		condition: sync.NewCond(&sync.Mutex{}),
	}

	// 使用原子操作初始化
	atomic.StoreInt64(&rl.readTokens, readBytesPerSecond)
	atomic.StoreInt64(&rl.writeTokens, writeBytesPerSecond)
	atomic.StoreInt64(&rl.lastUpdate, time.Now().UnixNano())

	return rl
}

// WaitRead 等待读取令牌
func (rl *RateLimiter) WaitRead(bytes int64) {
	if rl == nil || bytes <= 0 {
		return
	}
	rl.waitTokens(bytes, &rl.readTokens)
}

// WaitWrite 等待写入令牌
func (rl *RateLimiter) WaitWrite(bytes int64) {
	if rl == nil || bytes <= 0 {
		return
	}
	rl.waitTokens(bytes, &rl.writeTokens)
}

// SetRate 动态调整读写速率
func (rl *RateLimiter) SetRate(readBytesPerSecond, writeBytesPerSecond int64) {
	if rl == nil {
		return
	}

	// 如果某个速率为0，设置为无限制
	if readBytesPerSecond <= 0 {
		readBytesPerSecond = 1 << 40 // 1TB/s
	}
	if writeBytesPerSecond <= 0 {
		writeBytesPerSecond = 1 << 40 // 1TB/s
	}

	rl.condition.L.Lock()
	defer rl.condition.L.Unlock()

	atomic.StoreInt64(&rl.readRate, readBytesPerSecond)
	atomic.StoreInt64(&rl.writeRate, writeBytesPerSecond)

	// 重新填充令牌桶
	atomic.StoreInt64(&rl.readTokens, readBytesPerSecond)
	atomic.StoreInt64(&rl.writeTokens, writeBytesPerSecond)

	// 唤醒所有等待的协程
	rl.condition.Broadcast()
}

// Reset 重置限速器状态
func (rl *RateLimiter) Reset() {
	if rl == nil {
		return
	}

	rl.condition.L.Lock()
	defer rl.condition.L.Unlock()

	// 重置令牌数量为当前速率
	readRate := atomic.LoadInt64(&rl.readRate)
	writeRate := atomic.LoadInt64(&rl.writeRate)

	atomic.StoreInt64(&rl.readTokens, readRate)
	atomic.StoreInt64(&rl.writeTokens, writeRate)
	atomic.StoreInt64(&rl.lastUpdate, time.Now().UnixNano())

	rl.condition.Broadcast()
}

// waitTokens 等待令牌
func (rl *RateLimiter) waitTokens(bytes int64, tokens *int64) {
	if rl == nil || bytes <= 0 {
		return
	}

	rl.condition.L.Lock()
	defer rl.condition.L.Unlock()
	for {
		rl.refillTokens()

		// 原子获取并消耗令牌
		if curr := atomic.LoadInt64(tokens); curr >= bytes &&
			atomic.CompareAndSwapInt64(tokens, curr, curr-bytes) {
			return
		}
		// 等待令牌
		rl.condition.Wait()
	}
}

// refillTokens 更新令牌
func (rl *RateLimiter) refillTokens() {
	now := time.Now().UnixNano()
	last := atomic.LoadInt64(&rl.lastUpdate)

	if elapsed := now - last; elapsed > 0 && atomic.CompareAndSwapInt64(&rl.lastUpdate, last, now) {
		// 更新读写令牌
		elapsedSeconds := float64(elapsed) / float64(time.Second)
		readAdd := int64(float64(rl.readRate) * elapsedSeconds)
		writeAdd := int64(float64(rl.writeRate) * elapsedSeconds)

		rl.addTokens(&rl.readTokens, readAdd, rl.readRate)
		rl.addTokens(&rl.writeTokens, writeAdd, rl.writeRate)

		// 通知等待的协程
		rl.condition.Broadcast()
	}
}

// addTokens 添加令牌
func (rl *RateLimiter) addTokens(tokens *int64, add, max int64) {
	if add <= 0 {
		return
	}
	for {
		curr := atomic.LoadInt64(tokens)
		newVal := min(curr+add, max)
		if atomic.CompareAndSwapInt64(tokens, curr, newVal) {
			break
		}
	}
}

// StatConn 是一个包装了 net.Conn 的结构体，用于统计并限制读取和写入的字节数
type StatConn struct {
	Conn net.Conn
	RX   *uint64
	TX   *uint64
	Rate *RateLimiter
}

// NewStatConn 创建一个新的 StatConn
func NewStatConn(conn net.Conn, rx, tx *uint64, rate *RateLimiter) *StatConn {
	return &StatConn{
		Conn: conn,
		RX:   rx,
		TX:   tx,
		Rate: rate,
	}
}

// Read 实现了 io.Reader 接口，读取数据时会统计读取字节数并进行限速
func (sc *StatConn) Read(b []byte) (int, error) {
	n, err := sc.Conn.Read(b)
	if n > 0 {
		atomic.AddUint64(sc.RX, uint64(n))
		if sc.Rate != nil {
			sc.Rate.WaitRead(int64(n))
		}
	}
	return n, err
}

// Write 实现了 io.Writer 接口，写入数据时会统计写入字节数并进行限速
func (sc *StatConn) Write(b []byte) (int, error) {
	if sc.Rate != nil {
		sc.Rate.WaitWrite(int64(len(b)))
	}
	n, err := sc.Conn.Write(b)
	if n > 0 {
		atomic.AddUint64(sc.TX, uint64(n))
	}
	return n, err
}

// Close 关闭连接
func (sc *StatConn) Close() error {
	return sc.Conn.Close()
}

// LocalAddr 返回本地地址
func (sc *StatConn) LocalAddr() net.Addr {
	return sc.Conn.LocalAddr()
}

// RemoteAddr 返回远程地址
func (sc *StatConn) RemoteAddr() net.Addr {
	return sc.Conn.RemoteAddr()
}

// SetDeadline 设置连接的读写超时
func (sc *StatConn) SetDeadline(t time.Time) error {
	return sc.Conn.SetDeadline(t)
}

// SetReadDeadline 设置连接的读取超时
func (sc *StatConn) SetReadDeadline(t time.Time) error {
	return sc.Conn.SetReadDeadline(t)
}

// SetWriteDeadline 设置连接的写入超时
func (sc *StatConn) SetWriteDeadline(t time.Time) error {
	return sc.Conn.SetWriteDeadline(t)
}

// GetConn 返回底层的 net.Conn
func (sc *StatConn) GetConn() net.Conn {
	return sc.Conn
}

// GetRate 返回当前的限速
func (sc *StatConn) GetRate() *RateLimiter {
	return sc.Rate
}

// GetRX 返回已接收的字节数
func (sc *StatConn) GetRX() uint64 {
	return atomic.LoadUint64(sc.RX)
}

// GetTX 返回已发送的字节数
func (sc *StatConn) GetTX() uint64 {
	return atomic.LoadUint64(sc.TX)
}

// GetTotal 返回总的传输字节数
func (sc *StatConn) GetTotal() uint64 {
	return sc.GetRX() + sc.GetTX()
}

// Reset 重置统计数据
func (sc *StatConn) Reset() {
	atomic.StoreUint64(sc.RX, 0)
	atomic.StoreUint64(sc.TX, 0)
}

// AsTCPConn 安全地将底层连接转换为 *net.TCPConn
func (sc *StatConn) AsTCPConn() (*net.TCPConn, bool) {
	if tcpConn, ok := sc.Conn.(*net.TCPConn); ok {
		return tcpConn, true
	}
	return nil, false
}

// AsUDPConn 安全地将底层连接转换为 *net.UDPConn
func (sc *StatConn) AsUDPConn() (*net.UDPConn, bool) {
	if udpConn, ok := sc.Conn.(*net.UDPConn); ok {
		return udpConn, true
	}
	return nil, false
}

// SetKeepAlive 设置TCP连接的KeepAlive状态
func (sc *StatConn) SetKeepAlive(keepalive bool) error {
	if tcpConn, ok := sc.AsTCPConn(); ok {
		return tcpConn.SetKeepAlive(keepalive)
	}
	return &net.OpError{Op: "set", Net: "tcp", Err: ErrNotTCPConn}
}

// SetKeepAlivePeriod 设置TCP连接的KeepAlive周期
func (sc *StatConn) SetKeepAlivePeriod(d time.Duration) error {
	if tcpConn, ok := sc.AsTCPConn(); ok {
		return tcpConn.SetKeepAlivePeriod(d)
	}
	return &net.OpError{Op: "set", Net: "tcp", Err: ErrNotTCPConn}
}

// SetNoDelay 设置TCP连接的NoDelay状态（禁用/启用Nagle算法）
func (sc *StatConn) SetNoDelay(noDelay bool) error {
	if tcpConn, ok := sc.AsTCPConn(); ok {
		return tcpConn.SetNoDelay(noDelay)
	}
	return &net.OpError{Op: "set", Net: "tcp", Err: ErrNotTCPConn}
}

// SetLinger 设置TCP连接的Linger时间
func (sc *StatConn) SetLinger(sec int) error {
	if tcpConn, ok := sc.AsTCPConn(); ok {
		return tcpConn.SetLinger(sec)
	}
	return &net.OpError{Op: "set", Net: "tcp", Err: ErrNotTCPConn}
}

// CloseRead 关闭TCP连接的读取端
func (sc *StatConn) CloseRead() error {
	if tcpConn, ok := sc.AsTCPConn(); ok {
		return tcpConn.CloseRead()
	}
	return &net.OpError{Op: "close", Net: "tcp", Err: ErrNotTCPConn}
}

// CloseWrite 关闭TCP连接的写入端
func (sc *StatConn) CloseWrite() error {
	if tcpConn, ok := sc.AsTCPConn(); ok {
		return tcpConn.CloseWrite()
	}
	return &net.OpError{Op: "close", Net: "tcp", Err: ErrNotTCPConn}
}

// ReadFromUDP 从UDP连接读取数据包，返回数据和发送方地址
func (sc *StatConn) ReadFromUDP(b []byte) (int, *net.UDPAddr, error) {
	if udpConn, ok := sc.AsUDPConn(); ok {
		n, addr, err := udpConn.ReadFromUDP(b)
		if n > 0 {
			atomic.AddUint64(sc.RX, uint64(n))
			if sc.Rate != nil {
				sc.Rate.WaitRead(int64(n))
			}
		}
		return n, addr, err
	}
	return 0, nil, &net.OpError{Op: "read", Net: "udp", Err: ErrNotUDPConn}
}

// WriteToUDP 向指定UDP地址发送数据包
func (sc *StatConn) WriteToUDP(b []byte, addr *net.UDPAddr) (int, error) {
	if udpConn, ok := sc.AsUDPConn(); ok {
		if sc.Rate != nil {
			sc.Rate.WaitWrite(int64(len(b)))
		}
		n, err := udpConn.WriteToUDP(b, addr)
		if n > 0 {
			atomic.AddUint64(sc.TX, uint64(n))
		}
		return n, err
	}
	return 0, &net.OpError{Op: "write", Net: "udp", Err: ErrNotUDPConn}
}

// ReadMsgUDP 从UDP连接读取消息，支持读取控制信息
func (sc *StatConn) ReadMsgUDP(b, oob []byte) (n, oobn, flags int, addr *net.UDPAddr, err error) {
	if udpConn, ok := sc.AsUDPConn(); ok {
		n, oobn, flags, addr, err = udpConn.ReadMsgUDP(b, oob)
		if n > 0 {
			atomic.AddUint64(sc.RX, uint64(n))
			if sc.Rate != nil {
				sc.Rate.WaitRead(int64(n))
			}
		}
		return n, oobn, flags, addr, err
	}
	return 0, 0, 0, nil, &net.OpError{Op: "read", Net: "udp", Err: ErrNotUDPConn}
}

// WriteMsgUDP 向UDP连接发送消息，支持发送控制信息
func (sc *StatConn) WriteMsgUDP(b, oob []byte, addr *net.UDPAddr) (n, oobn int, err error) {
	if udpConn, ok := sc.AsUDPConn(); ok {
		if sc.Rate != nil {
			sc.Rate.WaitWrite(int64(len(b)))
		}
		n, oobn, err = udpConn.WriteMsgUDP(b, oob, addr)
		if n > 0 {
			atomic.AddUint64(sc.TX, uint64(n))
		}
		return n, oobn, err
	}
	return 0, 0, &net.OpError{Op: "write", Net: "udp", Err: ErrNotUDPConn}
}

// SetReadBuffer 设置UDP连接的读取缓冲区大小
func (sc *StatConn) SetReadBuffer(bytes int) error {
	if udpConn, ok := sc.AsUDPConn(); ok {
		return udpConn.SetReadBuffer(bytes)
	}
	return &net.OpError{Op: "set", Net: "udp", Err: ErrNotUDPConn}
}

// SetWriteBuffer 设置UDP连接的写入缓冲区大小
func (sc *StatConn) SetWriteBuffer(bytes int) error {
	if udpConn, ok := sc.AsUDPConn(); ok {
		return udpConn.SetWriteBuffer(bytes)
	}
	return &net.OpError{Op: "set", Net: "udp", Err: ErrNotUDPConn}
}

// IsTCP 检查底层连接是否为TCP连接
func (sc *StatConn) IsTCP() bool {
	_, ok := sc.AsTCPConn()
	return ok
}

// IsUDP 检查底层连接是否为UDP连接
func (sc *StatConn) IsUDP() bool {
	_, ok := sc.AsUDPConn()
	return ok
}

// NetworkType 返回底层连接的网络类型
func (sc *StatConn) NetworkType() string {
	if sc.IsTCP() {
		return "tcp"
	} else if sc.IsUDP() {
		return "udp"
	}
	return "unknown"
}

// TimeoutReader 包装了 net.Conn，支持设置读取超时
type TimeoutReader struct {
	Conn    net.Conn
	Timeout time.Duration
}

// Read 实现了 io.Reader 接口，读取数据时会设置读取超时
func (tr *TimeoutReader) Read(b []byte) (int, error) {
	if tr.Timeout > 0 {
		tr.Conn.SetReadDeadline(time.Now().Add(tr.Timeout))
	}
	return tr.Conn.Read(b)
}

// DataExchange 实现两个双向数据交换，支持空闲超时, 自定义缓冲区
func DataExchange(conn1, conn2 net.Conn, idleTimeout time.Duration, buffer1, buffer2 []byte) error {
	// 连接有效性检查
	if conn1 == nil || conn2 == nil {
		return io.ErrUnexpectedEOF
	}

	var wg sync.WaitGroup
	errChan := make(chan error, 2)

	// 定义一个函数用于双向数据传输
	copyData := func(src, dst net.Conn, buffer []byte) {
		defer wg.Done()

		reader := &TimeoutReader{Conn: src, Timeout: idleTimeout}
		_, err := io.CopyBuffer(dst, reader, buffer)
		errChan <- err

		if idleTimeout == 0 {
			// 没有设置超时，传输结束后关闭写端
			if tcpConn, ok := dst.(*net.TCPConn); ok {
				tcpConn.CloseWrite()
			} else {
				dst.Close()
			}
		}
	}

	// 启动双向数据传输
	wg.Add(2)
	go copyData(conn1, conn2, buffer1)
	go copyData(conn2, conn1, buffer2)
	wg.Wait()
	close(errChan)

	// 清空超时设置
	if idleTimeout > 0 {
		conn1.SetReadDeadline(time.Time{})
		conn2.SetReadDeadline(time.Time{})
	}

	for err := range errChan {
		if err != nil {
			return err
		}
	}

	return io.EOF
}
