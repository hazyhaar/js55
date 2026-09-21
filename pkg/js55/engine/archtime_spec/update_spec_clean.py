import json

with open('../iifes.txt') as f:
    compute = f.readline().strip()
    f.readline()
    norm = f.readline().strip()

content = f"""package spec

#Binding: {{
	name: string
	tag:  "archtimeTagBox3SetFromBufferAttribute" | "archtimeTagVector3Set" | "archtimeTagBufferAttributeGetX" | "archtimeTagBufferAttributeGetY" | "archtimeTagBufferAttributeGetZ" | "archtimeTagComputeVertexNormals" | "archtimeTagNormalizeNormals"
	src:  string
}}
contract: close({{
	version: 2
	scope:   "whole compiled ordinary body; no outer lexical access; compiler TDZ token only"
	input:   "ordinary data slots; Float32Array; normalized=false; itemSize=3; exact count; aligned bounded fixed backing"
	output:  "ordinary min/max with writable own x/y/z and structurally proved set"
	dependencies: ["getX", "getY", "getZ", "min.set", "max.set"]
	unknown:           "generic bytecode"
	maxCallbackPoints: 4096
	largeAtomic:       "maxCallbackPoints limits calls with OnCheckpoint configured; otherwise exact backing length and remaining gas bound the call; check interruption again before publishing output"
	checkpoint:        "unchanged cadence; any crossing uses generic code, without native callbacks or prefix replay"
	gasBase:           2048
	gasPerPoint:       1024
}})

funcs: [...#Binding] & [
	{{
		name: "Box3.setFromBufferAttribute"
		tag:  "archtimeTagBox3SetFromBufferAttribute"
		src:  "function setFromBufferAttribute(t){{let e=1/0,n=1/0,i=1/0,r=-1/0,s=-1/0,a=-1/0;for(let o=0,l=t.count;o<l;o++){{const l=t.getX(o),c=t.getY(o),h=t.getZ(o);l<e&&(e=l),c<n&&(n=c),h<i&&(i=h),l>r&&(r=l),c>s&&(s=c),h>a&&(a=h)}}return this.min.set(e,n,i),this.max.set(r,s,a),this}}"
	}},
	{{
		name: "Vector3.set"
		tag:  "archtimeTagVector3Set"
		src:  "function set(t,e,n){{return void 0===n&&(n=this.z),this.x=t,this.y=e,this.z=n,this}}"
	}},
	{{
		name: "BufferAttribute.getX"
		tag:  "archtimeTagBufferAttributeGetX"
		src:  "function getX(t){{return this.array[t*this.itemSize]}}"
	}},
	{{
		name: "BufferAttribute.getY"
		tag:  "archtimeTagBufferAttributeGetY"
		src:  "function getY(t){{return this.array[t*this.itemSize+1]}}"
	}},
	{{
		name: "BufferAttribute.getZ"
		tag:  "archtimeTagBufferAttributeGetZ"
		src:  "function getZ(t){{return this.array[t*this.itemSize+2]}}"
	}},
	{{
		name: "BufferGeometry.computeVertexNormals"
		tag:  "archtimeTagComputeVertexNormals"
		src:  {json.dumps(compute)}
	}},
	{{
		name: "BufferGeometry.normalizeNormals"
		tag:  "archtimeTagNormalizeNormals"
		src:  {json.dumps(norm)}
	}}
]
"""
with open('spec.cue', 'w') as f:
    f.write(content)
