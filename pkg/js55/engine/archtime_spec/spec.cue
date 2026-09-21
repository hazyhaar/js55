package spec

#Binding: {
	name: string
	tag:  "archtimeTagBox3SetFromBufferAttribute" | "archtimeTagVector3Set" | "archtimeTagBufferAttributeGetX" | "archtimeTagBufferAttributeGetY" | "archtimeTagBufferAttributeGetZ" | "archtimeTagComputeVertexNormals" | "archtimeTagNormalizeNormals"
	src:  string
}
contract: close({
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
})

funcs: [...#Binding] & [
	{
		name: "Box3.setFromBufferAttribute"
		tag:  "archtimeTagBox3SetFromBufferAttribute"
		src:  "function setFromBufferAttribute(t){let e=1/0,n=1/0,i=1/0,r=-1/0,s=-1/0,a=-1/0;for(let o=0,l=t.count;o<l;o++){const l=t.getX(o),c=t.getY(o),h=t.getZ(o);l<e&&(e=l),c<n&&(n=c),h<i&&(i=h),l>r&&(r=l),c>s&&(s=c),h>a&&(a=h)}return this.min.set(e,n,i),this.max.set(r,s,a),this}"
	},
	{
		name: "Vector3.set"
		tag:  "archtimeTagVector3Set"
		src:  "function set(t,e,n){return void 0===n&&(n=this.z),this.x=t,this.y=e,this.z=n,this}"
	},
	{
		name: "BufferAttribute.getX"
		tag:  "archtimeTagBufferAttributeGetX"
		src:  "function getX(t){return this.array[t*this.itemSize]}"
	},
	{
		name: "BufferAttribute.getY"
		tag:  "archtimeTagBufferAttributeGetY"
		src:  "function getY(t){return this.array[t*this.itemSize+1]}"
	},
	{
		name: "BufferAttribute.getZ"
		tag:  "archtimeTagBufferAttributeGetZ"
		src:  "function getZ(t){return this.array[t*this.itemSize+2]}"
	},
	{
		name: "BufferGeometry.computeVertexNormals"
		tag:  "archtimeTagComputeVertexNormals"
		src:  "(function(){let v0,v1,v2,v3,v4,v5,v6,v7,v8,v9,v10,v11,v12,v13,v14,v15,v16,v17,v18,v19,v20,v21,v22,v23,v24,v25,v26,v27,v28,v29,v30,v31,v32,v33,v34,v35,v36,v37,v38,v39,v40,v41,v42,v43,v44,v45,v46,v47,v48,v49,v50,v51,v52,v53,v54,v55,v56,v57,v58,v59,v60,v61,v62,v63,v64,v65,v66,v67,v68,v69,v70,v71,v72,v73,v74,v75,v76,v77,Lt,v79,v80,v81,v82,v83,v84,v85,v86,v87,v88,v89,v90,v91,v92,v93,v94,v95,v96,v97,v98,v99,v100,v101,v102,v103,v104,v105,v106,v107,v108,v109,v110,v111,v112,v113,v114,v115,v116,v117,v118,v119,v120,v121,v122,v123,v124,v125,v126,v127,v128,v129,v130,v131,v132,v133,v134,v135,v136,v137,v138,v139,v140,v141,v142,v143,v144,v145,v146,v147,v148,v149,v150,v151,v152,v153,v154,v155,v156,sn;return function computeVertexNormals(){const t=this.index,e=this.getAttribute(\"position\");if(void 0!==e){let n=this.getAttribute(\"normal\");if(void 0===n)n=new sn(new Float32Array(3*e.count),3),this.setAttribute(\"normal\",n);else for(let t=0,e=n.count;t<e;t++)n.setXYZ(t,0,0,0);const i=new Lt,r=new Lt,s=new Lt,a=new Lt,o=new Lt,l=new Lt,c=new Lt,h=new Lt;if(t)for(let u=0,d=t.count;u<d;u+=3){const d=t.getX(u+0),p=t.getX(u+1),m=t.getX(u+2);i.fromBufferAttribute(e,d),r.fromBufferAttribute(e,p),s.fromBufferAttribute(e,m),c.subVectors(s,r),h.subVectors(i,r),c.cross(h),a.fromBufferAttribute(n,d),o.fromBufferAttribute(n,p),l.fromBufferAttribute(n,m),a.add(c),o.add(c),l.add(c),n.setXYZ(d,a.x,a.y,a.z),n.setXYZ(p,o.x,o.y,o.z),n.setXYZ(m,l.x,l.y,l.z)}else for(let t=0,a=e.count;t<a;t+=3)i.fromBufferAttribute(e,t+0),r.fromBufferAttribute(e,t+1),s.fromBufferAttribute(e,t+2),c.subVectors(s,r),h.subVectors(i,r),c.cross(h),n.setXYZ(t+0,c.x,c.y,c.z),n.setXYZ(t+1,c.x,c.y,c.z),n.setXYZ(t+2,c.x,c.y,c.z);this.normalizeNormals(),n.needsUpdate=!0}}})()"
	},
	{
		name: "BufferGeometry.normalizeNormals"
		tag:  "archtimeTagNormalizeNormals"
		src:  "(function(){let v0,v1,v2,v3,v4,v5,v6,v7,v8,v9,v10,v11,v12,v13,v14,v15,v16,v17,v18,v19,v20,v21,v22,v23,v24,v25,v26,v27,v28,v29,v30,v31,v32,v33,v34,v35,v36,v37,v38,v39,v40,v41,v42,v43,v44,v45,v46,v47,v48,v49,v50,v51,v52,v53,v54,v55,v56,v57,v58,v59,v60,v61,v62,v63,v64,v65,v66,v67,v68,v69,v70,v71,v72,v73,v74,v75,v76,v77,v78,v79,v80,v81,v82,v83,v84,v85,v86,v87,v88,v89,v90,v91,v92,v93,v94,v95,v96,v97,v98,v99,v100,v101,v102,v103,v104,v105,v106,v107,v108,v109,v110,v111,v112,v113,v114,v115,v116,v117,v118,v119,v120,v121,v122,v123,v124,v125,v126,v127,v128,v129,v130,v131,v132,v133,v134,v135,v136,v137,v138,v139,v140,v141,v142,v143,v144,v145,v146,v147,v148,v149,v150,v151,v152,v153,v154,v155,v156,v157,v158,v159,v160,v161,v162,v163,v164,v165,v166,v167,v168,v169,v170,v171,v172,v173,v174,Tn;return function normalizeNormals(){const t=this.attributes.normal;for(let e=0,n=t.count;e<n;e++)Tn.fromBufferAttribute(t,e),Tn.normalize(),t.setXYZ(e,Tn.x,Tn.y,Tn.z)}})()"
	},
]
