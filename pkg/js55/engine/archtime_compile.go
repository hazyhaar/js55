package engine

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math"
)

type archtimeGeometryTag uint8

const (
	archtimeTagNone archtimeGeometryTag = iota
	archtimeTagBox3SetFromBufferAttribute
	archtimeTagVector3Set
	archtimeTagBufferAttributeGetX
	archtimeTagBufferAttributeGetY
	archtimeTagBufferAttributeGetZ
	archtimeTagComputeVertexNormals
	archtimeTagNormalizeNormals
)

func hashChunk(c *Chunk) string {
	if c == nil || c.Native != nil || c.Generator || c.Async || c.Arrow || c.ExportDefault || len(c.ExportNames) != 0 {
		return ""
	}
	h := sha256.New()
	binary.Write(h, binary.LittleEndian, int64(len(c.Code)))
	h.Write(c.Code)

	binary.Write(h, binary.LittleEndian, int64(len(c.Consts)))
	for _, k := range c.Consts {
		binary.Write(h, binary.LittleEndian, uint8(k.Kind))
		switch k.Kind {
		case ConstNumber:
			binary.Write(h, binary.LittleEndian, math.Float64bits(k.Num))
		case ConstString:
			if k.Text == nil {
				return ""
			}
			str := k.Text.GoString()
			binary.Write(h, binary.LittleEndian, int64(len(str)))
			h.Write([]byte(str))
		case ConstFunction:
			return ""
		case ConstBigInt:
			binary.Write(h, binary.LittleEndian, k.Big)
		default:
			return ""
		}
	}

	binary.Write(h, binary.LittleEndian, int64(c.Locals))
	binary.Write(h, binary.LittleEndian, int64(c.Params))
	binary.Write(h, binary.LittleEndian, int64(c.SelfSlot))
	binary.Write(h, binary.LittleEndian, int64(c.ArgumentsSlot))
	binary.Write(h, binary.LittleEndian, int64(c.RestSlot))

	var flags uint8
	if c.Generator {
		flags |= 1
	}
	if c.Async {
		flags |= 2
	}
	if c.Strict {
		flags |= 4
	}
	if c.Arrow {
		flags |= 8
	}
	binary.Write(h, binary.LittleEndian, flags)

	binary.Write(h, binary.LittleEndian, int64(len(c.Handlers)))
	for _, hh := range c.Handlers {
		binary.Write(h, binary.LittleEndian, int64(hh.Start))
		binary.Write(h, binary.LittleEndian, int64(hh.End))
		binary.Write(h, binary.LittleEndian, int64(hh.CatchIP))
		binary.Write(h, binary.LittleEndian, int64(hh.FinallyIP))
		binary.Write(h, binary.LittleEndian, int64(hh.FinallyEnd))
		binary.Write(h, binary.LittleEndian, int64(hh.CatchSlot))
		binary.Write(h, binary.LittleEndian, int64(hh.Depth))
	}

	for ip := 0; ip < len(c.Code); {
		op := Op(c.Code[ip])
		if op.Width() < 0 || ip+1+op.Width() > len(c.Code) {
			return ""
		}
		switch op {
		case OpGetLocal, OpSetLocal:
			// depth is allowed as long as the hash captures it, enabling closed captures.

		case OpSetGlobal, OpGetGlobalSoft:
			return ""
		case OpGetGlobal:
			arg := binary.LittleEndian.Uint32(c.Code[ip+1 : ip+5])
			if int(arg) >= len(c.Consts) {
				return ""
			}
			k := c.Consts[arg]
			if k.Kind != ConstString || k.Text.GoString() != "\x00tdz" && k.Text.GoString() != "Float32Array" && k.Text.GoString() != "Math" {
				return ""
			}
		}
		ip += 1 + op.Width()
	}

	return hex.EncodeToString(h.Sum(nil))
}

func filterArchtimeCandidates(c *Chunk) {
	c.archtimeTag = archtimeTagNone
	c.archtimeHash = ""
	for _, n := range archtimeCodeLengths {
		if len(c.Code) == n {
			c.archtimeHash = hashChunk(c)
			recognizeArchtimeGeometry(c)
			return
		}
	}
}
