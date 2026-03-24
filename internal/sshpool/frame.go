package sshpool

import (
	"encoding/binary"
	"fmt"
	"io"
)

// 帧类型常量
const (
	FrameStdout   byte = 0x01 // 远程 stdout (S→C)
	FrameStderr   byte = 0x02 // 远程 stderr (S→C)
	FrameStdin    byte = 0x03 // 输入到远程 stdin (C→S)
	FrameExitCode byte = 0x04 // 退出码 4 bytes int32 BE (S→C)
	FrameResize   byte = 0x05 // 终端大小 4 bytes: uint16 cols + uint16 rows (C→S)
	FrameError    byte = 0x06 // 错误信息 UTF-8 (S→C)
	FrameFileData byte = 0x07 // 文件数据块 (双向)
	FrameFileEOF  byte = 0x08 // 文件传输结束 (双向)
	FrameFileErr  byte = 0x09 // 文件操作错误 (S→C)
	FrameOK       byte = 0x0A // 操作成功 (S→C)

	// MaxFramePayload 最大帧负载大小 64KB
	MaxFramePayload = 64 * 1024
)

// WriteFrame 写入一个帧到 writer
// 帧格式: [Length: 4 bytes uint32 BE] [Type: 1 byte] [Payload: Length-1 bytes]
func WriteFrame(w io.Writer, frameType byte, payload []byte) error {
	if len(payload) > MaxFramePayload {
		return fmt.Errorf("payload too large: %d > %d", len(payload), MaxFramePayload)
	}
	// Length = 1 (type) + len(payload)
	length := uint32(1 + len(payload))
	header := make([]byte, 5) // 4 bytes length + 1 byte type
	binary.BigEndian.PutUint32(header[:4], length)
	header[4] = frameType
	if _, err := w.Write(header); err != nil {
		return fmt.Errorf("write frame header: %w", err)
	}
	if len(payload) > 0 {
		if _, err := w.Write(payload); err != nil {
			return fmt.Errorf("write frame payload: %w", err)
		}
	}
	return nil
}

// ReadFrame 从 reader 读取一个帧
// 返回帧类型和负载
func ReadFrame(r io.Reader) (byte, []byte, error) {
	var header [5]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return 0, nil, err
	}
	length := binary.BigEndian.Uint32(header[:4])
	if length < 1 {
		return 0, nil, fmt.Errorf("invalid frame length: %d", length)
	}
	frameType := header[4]
	payloadLen := length - 1
	if payloadLen > MaxFramePayload {
		return 0, nil, fmt.Errorf("frame payload too large: %d > %d", payloadLen, MaxFramePayload)
	}
	var payload []byte
	if payloadLen > 0 {
		payload = make([]byte, payloadLen)
		if _, err := io.ReadFull(r, payload); err != nil {
			return 0, nil, fmt.Errorf("read frame payload: %w", err)
		}
	}
	return frameType, payload, nil
}

// WriteExitCode 写入退出码帧
func WriteExitCode(w io.Writer, code int) error {
	payload := make([]byte, 4)
	binary.BigEndian.PutUint32(payload, uint32(code))
	return WriteFrame(w, FrameExitCode, payload)
}

// WriteError 写入错误帧
func WriteError(w io.Writer, msg string) error {
	return WriteFrame(w, FrameError, []byte(msg))
}

// WriteResize 写入终端大小变更帧
func WriteResize(w io.Writer, cols, rows uint16) error {
	payload := make([]byte, 4)
	binary.BigEndian.PutUint16(payload[:2], cols)
	binary.BigEndian.PutUint16(payload[2:], rows)
	return WriteFrame(w, FrameResize, payload)
}

// ParseExitCode 解析退出码帧负载
func ParseExitCode(payload []byte) (int, error) {
	if len(payload) != 4 {
		return 0, fmt.Errorf("invalid exit code payload length: %d", len(payload))
	}
	return int(int32(binary.BigEndian.Uint32(payload))), nil
}

// ParseResize 解析终端大小帧负载
func ParseResize(payload []byte) (cols, rows uint16, err error) {
	if len(payload) != 4 {
		return 0, 0, fmt.Errorf("invalid resize payload length: %d", len(payload))
	}
	cols = binary.BigEndian.Uint16(payload[:2])
	rows = binary.BigEndian.Uint16(payload[2:])
	return cols, rows, nil
}
