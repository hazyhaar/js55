// SPDX-License-Identifier: BUSL-1.1
package engine

import (
	"encoding/json"
	"github.com/hazyhaar/js55/pkg/js55/parser"
	"strings"
	"testing"
)

// Exact whole bodies from the local Three r128 bundle, solely for compiler
// recognition tests. Runtime parity uses the unchanged full bundle separately.
const vec3SetCode = `function set(t,e,n){return void 0===n&&(n=this.z),this.x=t,this.y=e,this.z=n,this}`
const setFromBufferAttributeCode = `function setFromBufferAttribute(t){let e=1/0,n=1/0,i=1/0,r=-1/0,s=-1/0,a=-1/0;for(let o=0,l=t.count;o<l;o++){const l=t.getX(o),c=t.getY(o),h=t.getZ(o);l<e&&(e=l),c<n&&(n=c),h<i&&(i=h),l>r&&(r=l),c>s&&(s=c),h>a&&(a=h)}return this.min.set(e,n,i),this.max.set(r,s,a),this}`

func archtimeCompileBody(t *testing.T, src string, strict bool) *Chunk {
	t.Helper()
	p, e := parser.Parse(src, parser.Options{})
	if e != nil {
		t.Fatal(e)
	}
	c, e := CompileMode(NewHeap(), p, "proof", strict)
	if e != nil {
		t.Fatal(e)
	}
	for _, k := range c.Consts {
		if k.Kind == ConstFunction {
			return k.Fn
		}
	}
	t.Fatal("missing function")
	return nil
}

func TestArchtimeRecognition(t *testing.T) {
	for _, strict := range []bool{false, true} {
		for _, tc := range []struct {
			src string
			tag archtimeGeometryTag
		}{
			{setFromBufferAttributeCode, archtimeTagBox3SetFromBufferAttribute},
			{vec3SetCode, archtimeTagVector3Set},
			{strings.Replace(setFromBufferAttributeCode, "setFromBufferAttribute", "alias", 1), archtimeTagBox3SetFromBufferAttribute},
			{`function setFromBufferAttribute(t){return 123}`, archtimeTagNone},
			{strings.Replace(setFromBufferAttributeCode, "e=1/0", "e=1/0.00000001", 1), archtimeTagNone},
			{strings.Replace(setFromBufferAttributeCode, "e=1/0", "e=outside", 1), archtimeTagNone},
			{`function getY(t){return this.array[t*this.itemSize+1.00000001]}`, archtimeTagNone},
		} {
			c := archtimeCompileBody(t, tc.src, strict)
			if c.archtimeTag != tc.tag {
				t.Errorf("tag %d expected %d strict=%t for %s", c.archtimeTag, tc.tag, strict, tc.src)
			}
		}
	}
}

func TestArchtimeFingerprintMutationAndSerialization(t *testing.T) {
	c := archtimeCompileBody(t, vec3SetCode, false)
	saved := c.archtimeHash
	c.Params++
	if hashChunk(c) == saved {
		t.Fatal("parameter metadata not covered")
	}
	c.Params--
	c.Code = append(c.Code, byte(OpReturn))
	if hashChunk(c) == saved {
		t.Fatal("whole code not covered")
	}
	c.Code = c.Code[:len(c.Code)-1]
	if hashChunk(c) != saved {
		t.Fatal("restored code did not recover")
	}
	original := c.Consts[0]
	c.Consts[0] = Const{Kind: ConstNumber, Num: 1.00000001}
	h1 := hashChunk(c)
	c.Consts[0].Num = 1.00000002
	if hashChunk(c) == h1 {
		t.Fatal("rounded constant collision")
	}
	c.Consts[0] = original
	if hashChunk(c) != saved {
		t.Fatal("restored constants did not recover")
	}
	// Public input cannot reconstruct a private compiler proof.
	var copy Chunk
	data := []byte(`{"Name":"set","archtimeTag":2,"archtimeHash":"forged"}`)
	if err := json.Unmarshal(data, &copy); err != nil {
		t.Fatal(err)
	}
	if copy.archtimeTag != archtimeTagNone || copy.archtimeHash != "" {
		t.Fatal("public deserialization forged proof")
	}
}
