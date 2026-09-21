import re
import json

with open('../iifes.txt') as f:
    compute = f.readline().strip()
    f.readline()
    norm = f.readline().strip()

with open('spec.cue', 'r') as f:
    cue = f.read()

cue = re.sub(r'tag:\s*"archtimeTagComputeVertexNormals"\s*\n\s*src:\s*".*?"', f'tag:  "archtimeTagComputeVertexNormals"\n\t\tsrc:  {json.dumps(compute)}', cue)
cue = re.sub(r'tag:\s*"archtimeTagNormalizeNormals"\s*\n\s*src:\s*".*?"', f'tag:  "archtimeTagNormalizeNormals"\n\t\tsrc:  {json.dumps(norm)}', cue)

with open('spec.cue', 'w') as f:
    f.write(cue)
