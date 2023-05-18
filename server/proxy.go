package main

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
)

const socks5Ver = 0x05
const cmdBind = 0x01
const atypeIPV4 = 0x01
const atypeHOST = 0x03
const atypeIPV6 = 0x04

func main() {
	server, err := net.Listen("tcp", "127.0.0.1:1080")
	if err != nil {
		panic(err)
	}
	for {
		client, err := server.Accept()
		if err != nil {
			log.Printf("Accept error: %s", err)
			continue
		}
		// fmt.Println("client", client)
		go process(client) // 相当于子线程
	}
}

func process(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	err := auth(reader, conn)
	if err != nil {
		log.Printf("client %v auth failed: %v", conn.RemoteAddr(), err)
		return
	}
	err = connect(reader, conn)
	if err != nil {
		log.Printf("client %v auth failed: %v", conn.RemoteAddr(), err)
		return
	}
}

func auth(reader *bufio.Reader, conn net.Conn) (err error) {
	// +----+----------+----------+
	// |VER | NMETHODS | METHODS  |
	// +----+----------+----------+
	// | 1  |    1     | 1 to 255 |
	// +----+----------+----------+
	// VER: 协议版本，socks5为0x05
	// NMETHODS: 支持认证的方法数量
	// METHODS: 对应NMETHODS，NMETHODS的值为多少，METHODS就有多少个字节。RFC预定义了一些值的含义，内容如下:
	// X’00’ NO AUTHENTICATION REQUIRED
	// X’02’ USERNAME/PASSWORD

	ver, err := reader.ReadByte() // 读取版本
	if err != nil {
		return fmt.Errorf("read version failed: %w", err)
	}
	if ver != socks5Ver {
		return fmt.Errorf("not supported version: %v", ver)
	}
	methodSize, err := reader.ReadByte() // 读取方法数量
	if err != nil {
		return fmt.Errorf("read methodSize failed: %w", err)
	}
	method := make([]byte, methodSize) // 读取方法
	_, err = io.ReadFull(reader, method)
	if err != nil {
		return fmt.Errorf("read method failed: %w", err)
	}
	log.Println("ver", ver, "method", method)
	// +----+--------+
	// |VER | METHOD |
	// +----+--------+
	// | 1  |   1    |
	// +----+--------+
	_, err = conn.Write([]byte{socks5Ver, 0x00}) // 回复客户端，无需认证
	if err != nil {
		return fmt.Errorf("write failed: %w", err)
	}
	return nil
}

func connect(reader *bufio.Reader, conn net.Conn) (err error) {
	// +----+-----+-------+------+----------+----------+
	// |VER | CMD |  RSV  | ATYP | DST.ADDR | DST.PORT |
	// +----+-----+-------+------+----------+----------+
	// | 1  |  1  | X'00' |  1   | Variable |    2     |
	// +----+-----+-------+------+----------+----------+
	// VER 版本号，socks5的值为0x05
	// CMD 0x01表示CONNECT请求（目前仅支持CONNECT请求）
	// RSV 保留字段，值为0x00
	// ATYP 目标地址类型，DST.ADDR的数据对应这个字段的类型。
	//   0x01表示IPv4地址，DST.ADDR为4个字节
	//   0x03表示域名，DST.ADDR是一个可变长度的域名
	// DST.ADDR 一个可变长度的值
	// DST.PORT 目标端口，固定2个字节

	buf := make([]byte, 4) // 读取前面固定长度的四个字节
	_, err = io.ReadFull(reader, buf)
	if err != nil {
		return fmt.Errorf("read header failed: %w", err)
	}
	ver, cmd, atype := buf[0], buf[1], buf[3]
	if ver != socks5Ver {
		return fmt.Errorf("not supported ver: %v", ver)
	}
	if cmd != cmdBind {
		return fmt.Errorf("not supported cmd: %v", cmd)
	}
	addr := ""
	switch atype {
	case atypeIPV4: // 读取IPV4地址，固定长度四个字节
		_, err = io.ReadFull(reader, buf)
		if err != nil {
			return fmt.Errorf("read atyp failed: %w", err)
		}
		// addr = fmt.Sprintf("%d.%d.%d.%d", buf[0], buf[1], buf[2], buf[3])
		addr = net.IPv4(buf[0], buf[1], buf[2], buf[3]).String()
	case atypeHOST:
		hostSize, err := reader.ReadByte() // 读取域名长度
		if err != nil {
			return fmt.Errorf("read hostSize failed: %w", err)
		}
		host := make([]byte, hostSize)
		_, err = io.ReadFull(reader, host)
		if err != nil {
			return fmt.Errorf("read host failed: %w", err)
		}
		addr = string(host)
	case atypeIPV6:
		bufIPv6 := make([]byte, 16)
		_, err = io.ReadFull(reader, bufIPv6)
		if err != nil {
			return fmt.Errorf("read ipv6 failed: %w", err)
		}
		addr = net.IP(bufIPv6).String()
	default:
		return errors.New("invalid atyp")
	}
	_, err = io.ReadFull(reader, buf[:2]) // 读取端口，固定长度两个字节
	if err != nil {
		return fmt.Errorf("read port failed: %w", err)
	}
	port := binary.BigEndian.Uint16(buf[:2]) // 端口是大端序

	dest, err := net.Dial("tcp", fmt.Sprintf("[%s]:%d", addr, port)) // tcp连接目标地址
	if err != nil {
		return fmt.Errorf("dial failed: %w", err)
	}
	defer dest.Close()

	log.Println("dial", addr, port)

	// +----+-----+-------+------+----------+----------+
	// |VER | REP |  RSV  | ATYP | BND.ADDR | BND.PORT |
	// +----+-----+-------+------+----------+----------+
	// | 1  |  1  | X'00' |  1   | Variable |    2     |
	// +----+-----+-------+------+----------+----------+
	// VER socks版本，这里为0x05
	// REP Relay field,内容取值如下 X’00’ succeeded
	// RSV 保留字段
	// ATYPE 地址类型
	// BND.ADDR 服务绑定的地址
	// BND.PORT 服务绑定的端口DST.PORT
	_, err = conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0}) // 回复客户端，连接成功
	if err != nil {
		return fmt.Errorf("write failed: %w", err)
	}

	// 进行转发
	cxt, cancel := context.WithCancel(context.Background())
	defer cancel() // 防御式编程，没有意义
	go func() {
		_, _ = io.Copy(dest, reader) // 从客户端读取数据，转发给目标地址
		cancel()
	}()
	go func() {
		_, _ = io.Copy(conn, dest) // 从目标地址读取数据，转发给客户端
		cancel()
	}()
	<-cxt.Done() // 阻塞，直到cancel被调用
	return nil
}
