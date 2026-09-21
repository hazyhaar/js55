Edit `/devhoros/pkg/js55/engine/vm.go` fonction `makeArguments`.

Remplacer :

```
func (vm *VM) makeArguments(argc int) Value {
	h := vm.heap.NewObject()
	hv := ObjectValue(h)
	vm.heap.AddRoot(&hv)
	defer vm.heap.RemoveRoot(&hv)
	for i := 0; i < argc; i++ {
		v := vm.peek(argc - 1 - i)
		vm.heap.SetElement(h, i, v)
		vm.heap.SetProperty(h, vm.heap.Intern().InternGo(strconv.Itoa(i)), v)
	}
	vm.heap.SetProperty(h, vm.heap.Intern().InternGo("length"), Int(int32(argc)))
	return hv
}
```

par le même texte plus, avant `return hv` :

```
	vm.heap.SetProperty(h, vm.heap.Intern().InternGo("callee"), vm.peek(argc))
```

`peek(argc)` est le callee (sous les argc arguments). Un Edit. Interdit Read/Grep/Glob. Pas de test dans cette fiche.
