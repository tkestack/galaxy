package keylock

import "hash/crc32"

type KeyGen func(data []byte, len int) uint32

func Crc32Mod(data []byte, len int) uint32 {
	return crc32.ChecksumIEEE(data) % uint32(len)
}
