// Package pool 实现了一个高性能、可靠的网络连接池管理系统
package pool

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultMinCap           = 1
	defaultMaxCap           = 1
	defaultMinIvl           = 1 * time.Second
	defaultMaxIvl           = 1 * time.Second
	idReadTimeout           = 1 * time.Minute
	idRetryInterval         = 50 * time.Millisecond
	acceptRetryInterval     = 50 * time.Millisecond
	intervalAdjustStep      = 100 * time.Millisecond
	capacityAdjustLowRatio  = 0.2
	capacityAdjustHighRatio = 0.8
	intervalLowThreshold    = 0.2
	intervalHighThreshold   = 0.8
)

// Pool 连接池结构体，用于管理多个网络连接
type Pool struct {
	conns     sync.Map                 // 存储连接的映射表
	idChan    chan string              // 可用ID通道
	tlsCode   string                   // TLS安全模式代码
	hostname  string                   // 主机名
	clientIP  string                   // 客户端IP
	tlsConfig *tls.Config              // TLS配置
	dialer    func() (net.Conn, error) // 创建连接的函数
	listener  net.Listener             // 监听器
	errCount  atomic.Int32             // 错误计数
	capacity  atomic.Int32             // 当前容量
	minCap    int                      // 最小容量
	maxCap    int                      // 最大容量
	interval  atomic.Int64             // 连接创建间隔
	minIvl    time.Duration            // 最小间隔
	maxIvl    time.Duration            // 最大间隔
	keepAlive time.Duration            // 保活间隔
	ctx       context.Context          // 上下文
	cancel    context.CancelFunc       // 取消函数
}

// NewClientPool 创建新的客户端连接池
func NewClientPool(
	minCap, maxCap int,
	minIvl, maxIvl time.Duration,
	keepAlive time.Duration,
	tlsCode string,
	hostname string,
	dialer func() (net.Conn, error),
) *Pool {
	if minCap <= 0 {
		minCap = defaultMinCap
	}
	if maxCap <= 0 {
		maxCap = defaultMaxCap
	}
	if minCap > maxCap {
		minCap, maxCap = maxCap, minCap
	}

	if minIvl <= 0 {
		minIvl = defaultMinIvl
	}
	if maxIvl <= 0 {
		maxIvl = defaultMaxIvl
	}
	if minIvl > maxIvl {
		minIvl, maxIvl = maxIvl, minIvl
	}

	pool := &Pool{
		conns:     sync.Map{},
		idChan:    make(chan string, maxCap),
		tlsCode:   tlsCode,
		hostname:  hostname,
		dialer:    dialer,
		minCap:    minCap,
		maxCap:    maxCap,
		minIvl:    minIvl,
		maxIvl:    maxIvl,
		keepAlive: keepAlive,
	}
	pool.capacity.Store(int32(minCap))
	pool.interval.Store(int64(minIvl))
	pool.ctx, pool.cancel = context.WithCancel(context.Background())
	return pool
}

// NewServerPool 创建新的服务端连接池
func NewServerPool(
	maxCap int,
	clientIP string,
	tlsConfig *tls.Config,
	listener net.Listener,
	keepAlive time.Duration,
) *Pool {
	if maxCap <= 0 {
		maxCap = defaultMaxCap
	}

	if listener == nil {
		return nil
	}

	pool := &Pool{
		conns:     sync.Map{},
		idChan:    make(chan string, maxCap),
		clientIP:  clientIP,
		tlsConfig: tlsConfig,
		listener:  listener,
		maxCap:    maxCap,
		keepAlive: keepAlive,
	}
	pool.ctx, pool.cancel = context.WithCancel(context.Background())
	return pool
}

// createConnection 创建新的客户端连接
func (p *Pool) createConnection() bool {
	conn, err := p.dialer()
	if err != nil {
		return false
	}

	conn.(*net.TCPConn).SetKeepAlive(true)
	conn.(*net.TCPConn).SetKeepAlivePeriod(p.keepAlive)

	var id string
	// 根据TLS代码应用不同级别的TLS安全
	switch p.tlsCode {
	case "0":
		// 不使用TLS
	case "1":
		// 使用自签名证书（不验证）
		tlsConn := tls.Client(conn, &tls.Config{
			InsecureSkipVerify: true,
			MinVersion:         tls.VersionTLS13,
		})
		if err := tlsConn.Handshake(); err != nil {
			conn.Close()
			return false
		}
		conn = tlsConn
	case "2":
		// 使用验证证书（安全模式）
		tlsConn := tls.Client(conn, &tls.Config{
			InsecureSkipVerify: false,
			MinVersion:         tls.VersionTLS13,
			ServerName:         p.hostname,
		})
		if err := tlsConn.Handshake(); err != nil {
			conn.Close()
			return false
		}
		conn = tlsConn
	}

	// 接收连接ID
	conn.SetReadDeadline(time.Now().Add(idReadTimeout))
	buf := make([]byte, 4)
	n, err := io.ReadFull(conn, buf)
	if err != nil || n != 4 {
		conn.Close()
		return false
	}
	id = hex.EncodeToString(buf)
	conn.SetReadDeadline(time.Time{})

	// 建立映射并存入通道
	p.conns.Store(id, conn)
	select {
	case p.idChan <- id:
		return true
	default:
		p.conns.Delete(id)
		conn.Close()
		return false
	}
}

// handleConnection 处理新的服务端连接
func (p *Pool) handleConnection(conn net.Conn) {
	defer func() {
		if conn != nil {
			conn.Close()
		}
	}()

	// 检查池是否已满
	if p.Active() >= p.maxCap {
		return
	}

	conn.(*net.TCPConn).SetKeepAlive(true)
	conn.(*net.TCPConn).SetKeepAlivePeriod(p.keepAlive)

	// 验证客户端IP
	if p.clientIP != "" && conn.RemoteAddr().(*net.TCPAddr).IP.String() != p.clientIP {
		return
	}

	// 应用TLS
	if p.tlsConfig != nil {
		tlsConn := tls.Server(conn, p.tlsConfig)
		if err := tlsConn.Handshake(); err != nil {
			return
		}
		conn = tlsConn
	}

	// 生成连接ID
	rawID := make([]byte, 4)
	if _, err := rand.Read(rawID); err != nil {
		return
	}
	id := hex.EncodeToString(rawID)

	// 防止重复连接ID
	if _, exist := p.conns.Load(id); exist {
		return
	}

	// 发送ID给客户端并在成功后建立映射
	if _, err := conn.Write(rawID); err != nil {
		return
	}

	// 尝试放入idChan
	select {
	case p.idChan <- id:
		p.conns.Store(id, conn)
		conn = nil
	default:
		// 池满
		return
	}
}

// ClientManager 客户端连接池管理器
func (p *Pool) ClientManager() {
	if p.cancel != nil {
		p.cancel()
	}
	p.ctx, p.cancel = context.WithCancel(context.Background())

	for p.ctx.Err() == nil {
		p.adjustInterval()
		capacity := int(p.capacity.Load())
		need := capacity - len(p.idChan)
		created := 0

		if need > 0 {
			var wg sync.WaitGroup
			results := make(chan int, need)
			for range need {
				wg.Go(func() {
					if p.createConnection() {
						results <- 1
					}
				})
			}
			wg.Wait()
			close(results)
			for r := range results {
				created += r
			}
		}

		p.adjustCapacity(created)

		select {
		case <-p.ctx.Done():
			return
		case <-time.After(time.Duration(p.interval.Load())):
		}
	}
}

// ServerManager 服务端连接池管理器
func (p *Pool) ServerManager() {
	if p.cancel != nil {
		p.cancel()
	}
	p.ctx, p.cancel = context.WithCancel(context.Background())

	for p.ctx.Err() == nil {
		conn, err := p.listener.Accept()
		if err != nil {
			if p.ctx.Err() != nil || err == net.ErrClosed {
				return
			}

			select {
			case <-p.ctx.Done():
				return
			case <-time.After(acceptRetryInterval):
			}
			continue
		}

		go p.handleConnection(conn)
	}
}

// OutgoingGet 根据ID获取可用池连接
func (p *Pool) OutgoingGet(id string, timeout time.Duration) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(p.ctx, timeout)
	defer cancel()
	for {
		if conn, ok := p.conns.LoadAndDelete(id); ok {
			<-p.idChan
			return conn.(net.Conn), nil
		}
		select {
		case <-time.After(idRetryInterval):
		case <-ctx.Done():
			return nil, fmt.Errorf("OutgoingGet: pool connection not found")
		}
	}
}

// IncomingGet 获取可用池连接返回ID
func (p *Pool) IncomingGet(timeout time.Duration) (string, net.Conn, error) {
	ctx, cancel := context.WithTimeout(p.ctx, timeout)
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return "", nil, fmt.Errorf("IncomingGet: insufficient pool connections")
		case id := <-p.idChan:
			if conn, ok := p.conns.LoadAndDelete(id); ok {
				return id, conn.(net.Conn), nil
			}
			continue
		}
	}
}

// Flush 清空连接池中的所有连接
func (p *Pool) Flush() {
	var wg sync.WaitGroup
	p.conns.Range(func(key, value any) bool {
		wg.Go(func() {
			if value != nil {
				value.(net.Conn).Close()
			}
		})
		return true
	})
	wg.Wait()

	p.conns = sync.Map{}
	p.idChan = make(chan string, p.maxCap)
}

// Close 关闭连接池并释放资源
func (p *Pool) Close() {
	if p.cancel != nil {
		p.cancel()
	}
	p.Flush()
}

// Ready 检查连接池是否已初始化
func (p *Pool) Ready() bool {
	return p.ctx != nil
}

// Active 获取当前活跃连接数
func (p *Pool) Active() int {
	return len(p.idChan)
}

// Capacity 获取当前连接池容量
func (p *Pool) Capacity() int {
	return int(p.capacity.Load())
}

// Interval 获取当前连接创建间隔
func (p *Pool) Interval() time.Duration {
	return time.Duration(p.interval.Load())
}

// AddError 增加错误计数
func (p *Pool) AddError() {
	p.errCount.Add(1)
}

// ErrorCount 获取错误计数
func (p *Pool) ErrorCount() int {
	return int(p.errCount.Load())
}

// ResetError 重置错误计数
func (p *Pool) ResetError() {
	p.errCount.Store(0)
}

// adjustInterval 根据连接池使用情况动态调整连接创建间隔
func (p *Pool) adjustInterval() {
	idle := len(p.idChan)
	capacity := int(p.capacity.Load())
	interval := time.Duration(p.interval.Load())

	if idle < int(float64(capacity)*intervalLowThreshold) && interval > p.minIvl {
		newInterval := max(interval-intervalAdjustStep, p.minIvl)
		p.interval.Store(int64(newInterval))
	}

	if idle > int(float64(capacity)*intervalHighThreshold) && interval < p.maxIvl {
		newInterval := min(interval+intervalAdjustStep, p.maxIvl)
		p.interval.Store(int64(newInterval))
	}
}

// adjustCapacity 根据创建成功率动态调整连接池容量
func (p *Pool) adjustCapacity(created int) {
	capacity := int(p.capacity.Load())
	ratio := float64(created) / float64(capacity)

	if ratio < capacityAdjustLowRatio && capacity > p.minCap {
		p.capacity.Add(-1)
	}

	if ratio > capacityAdjustHighRatio && capacity < p.maxCap {
		p.capacity.Add(1)
	}
}
