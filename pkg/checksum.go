package pkg

type RollingChecksum struct {
	a uint32
	b uint32
	n int
}

// NewRollingChecksum initializes the rolling checksum over the given data.
func NewRollingChecksum(data []byte) *RollingChecksum {
	rc := &RollingChecksum{
		n: len(data),
	}
	for i, v := range data {
		rc.a += uint32(v)
		rc.b += uint32(rc.n-i) * uint32(v)
	}
	return rc
}

// Roll updates the rolling checksum as bytes move in and out of the block.
func (rc *RollingChecksum) Roll(out, in byte) {
	rc.a -= uint32(out)
	rc.a += uint32(in)
	rc.b -= uint32(rc.n) * uint32(out)
	rc.b += rc.a
}

// Sum returns the current value of the rolling checksum.
func (rc *RollingChecksum) Sum() uint32 {
	return (rc.a & 0xffff) | (rc.b&0xffff)<<16
}
