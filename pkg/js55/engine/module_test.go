// SPDX-License-Identifier: Apache-2.0 OR MIT

package engine

import (
	"testing"

	"github.com/hazyhaar/js55/pkg/js55/parser"
)

func compileAndRunModule(t *testing.T, src string) (*Chunk, *VM, Value, error) {
	t.Helper()
	prog, err := parser.Parse(src, parser.Options{Module: true})
	if err != nil {
		t.Fatalf("échec du parseur module pour %q : %v", src, err)
	}
	h := NewHeap()
	chunk, err := Compile(h, prog, "test_module")
	if err != nil {
		return nil, nil, Undefined, err
	}
	vm := NewVM(h)
	val, err := vm.Run(chunk)
	return chunk, vm, val, err
}

func TestModuleExportDefault(t *testing.T) {
	t.Run("ExpressionLitteraleEntier", func(t *testing.T) {
		src := `export default 42;`
		chunk, vm, val, err := compileAndRunModule(t, src)
		if err != nil {
			t.Fatalf("erreur inattendue : %v", err)
		}
		if !chunk.ExportDefault {
			t.Errorf("chunk.ExportDefault attendu true")
		}
		if !chunk.HasExport("default") {
			t.Errorf("chunk.HasExport(\"default\") attendu true")
		}
		defVal, ok := vm.ModuleExportDefault()
		if !ok {
			t.Fatalf("ModuleExportDefault attendu présent")
		}
		if !defVal.IsNumber() || defVal.ToInt() != 42 {
			t.Errorf("valeur exportée attendue 42, obtenu : %v", defVal)
		}
		if !val.IsNumber() || val.ToInt() != 42 {
			t.Errorf("valeur de complétion attendue 42, obtenu : %v", val)
		}
	})

	t.Run("ExpressionLitteraleChaine", func(t *testing.T) {
		src := `export default "bonjour";`
		_, vm, _, err := compileAndRunModule(t, src)
		if err != nil {
			t.Fatalf("erreur inattendue : %v", err)
		}
		defVal, ok := vm.GetExport("default")
		if !ok {
			t.Fatalf("GetExport(\"default\") attendu présent")
		}
		s := vm.StringOf(defVal)
		if s == nil || s.GoString() != "bonjour" {
			t.Errorf("valeur exportée attendue 'bonjour', obtenu : %v", defVal)
		}
	})

	t.Run("ExpressionIdentifiant", func(t *testing.T) {
		src := `const rep = 123; export default rep;`
		_, vm, _, err := compileAndRunModule(t, src)
		if err != nil {
			t.Fatalf("erreur inattendue : %v", err)
		}
		defVal, ok := vm.ModuleExportDefault()
		if !ok {
			t.Fatalf("ModuleExportDefault attendu présent")
		}
		if !defVal.IsNumber() || defVal.ToInt() != 123 {
			t.Errorf("valeur exportée attendue 123, obtenu : %v", defVal)
		}
	})

	t.Run("ExpressionObjet", func(t *testing.T) {
		src := `export default { a: 10, b: "val" };`
		_, vm, _, err := compileAndRunModule(t, src)
		if err != nil {
			t.Fatalf("erreur inattendue : %v", err)
		}
		defVal, ok := vm.ModuleExportDefault()
		if !ok || !defVal.IsObject() {
			t.Fatalf("ModuleExportDefault attendu objet présent, obtenu : %v", defVal)
		}
		propA := vm.getProp(defVal, vm.heap.Intern().InternGo("a"))
		if !propA.IsNumber() || propA.ToInt() != 10 {
			t.Errorf("prop 'a' attendue 10, obtenu : %v", propA)
		}
	})

	t.Run("ExpressionFonctionFlechee", func(t *testing.T) {
		src := `export default () => 99;`
		_, vm, _, err := compileAndRunModule(t, src)
		if err != nil {
			t.Fatalf("erreur inattendue : %v", err)
		}
		defVal, ok := vm.ModuleExportDefault()
		if !ok || !vm.isFunction(defVal) {
			t.Fatalf("ModuleExportDefault attendu fonction présent, obtenu : %v", defVal)
		}
		res, err := vm.invoke(Undefined, defVal, nil)
		if err != nil {
			t.Fatalf("échec d'invocation : %v", err)
		}
		if !res.IsNumber() || res.ToInt() != 99 {
			t.Errorf("résultat invocation attendu 99, obtenu : %v", res)
		}
	})

	t.Run("DeclarationFonctionNommee", func(t *testing.T) {
		src := `
			export default function foo() { return 1; }
			var res = foo();
		`
		_, vm, _, err := compileAndRunModule(t, src)
		if err != nil {
			t.Fatalf("erreur inattendue : %v", err)
		}
		defVal, ok := vm.ModuleExportDefault()
		if !ok || !vm.isFunction(defVal) {
			t.Fatalf("ModuleExportDefault attendu fonction")
		}
		resVal, _ := vm.GetGlobal("res")
		if !resVal.IsNumber() || resVal.ToInt() != 1 {
			t.Errorf("variable locale/globale 'res' attendue 1, obtenu : %v", resVal)
		}
		res, err := vm.invoke(Undefined, defVal, nil)
		if err != nil {
			t.Fatalf("échec d'invocation de default : %v", err)
		}
		if !res.IsNumber() || res.ToInt() != 1 {
			t.Errorf("résultat invocation attendu 1, obtenu : %v", res)
		}
	})

	t.Run("DeclarationFonctionAnonyme", func(t *testing.T) {
		src := `export default function() { return 2; }`
		_, vm, _, err := compileAndRunModule(t, src)
		if err != nil {
			t.Fatalf("erreur inattendue : %v", err)
		}
		defVal, ok := vm.ModuleExportDefault()
		if !ok || !vm.isFunction(defVal) {
			t.Fatalf("ModuleExportDefault attendu fonction")
		}
		res, err := vm.invoke(Undefined, defVal, nil)
		if err != nil {
			t.Fatalf("échec d'invocation de default : %v", err)
		}
		if !res.IsNumber() || res.ToInt() != 2 {
			t.Errorf("résultat invocation attendu 2, obtenu : %v", res)
		}
	})

	t.Run("DeclarationFonctionHoisting", func(t *testing.T) {
		src := `
			var avant = calcul();
			export default function calcul() { return 42; }
		`
		_, vm, _, err := compileAndRunModule(t, src)
		if err != nil {
			t.Fatalf("erreur inattendue : %v", err)
		}
		avantVal, _ := vm.GetGlobal("avant")
		if !avantVal.IsNumber() || avantVal.ToInt() != 42 {
			t.Errorf("hoisting attendu 42, obtenu : %v", avantVal)
		}
	})

	t.Run("DeclarationClasseNommee", func(t *testing.T) {
		src := `
			export default class Bar {
				val() { return 3; }
			}
			var inst = new Bar();
			var res = inst.val();
		`
		_, vm, _, err := compileAndRunModule(t, src)
		if err != nil {
			t.Fatalf("erreur inattendue : %v", err)
		}
		defVal, ok := vm.ModuleExportDefault()
		if !ok || !vm.isFunction(defVal) {
			t.Fatalf("ModuleExportDefault attendu constructeur de classe")
		}
		resVal, _ := vm.GetGlobal("res")
		if !resVal.IsNumber() || resVal.ToInt() != 3 {
			t.Errorf("résultat inst.val() attendu 3, obtenu : %v", resVal)
		}
	})

	t.Run("DeclarationClasseAnonyme", func(t *testing.T) {
		src := `
			export default class {
				val() { return 4; }
			}
		`
		_, vm, _, err := compileAndRunModule(t, src)
		if err != nil {
			t.Fatalf("erreur inattendue : %v", err)
		}
		defVal, ok := vm.ModuleExportDefault()
		if !ok || !vm.isFunction(defVal) {
			t.Fatalf("ModuleExportDefault attendu constructeur de classe")
		}
	})

	t.Run("ModuleExportsMap", func(t *testing.T) {
		src := `export default "ok";`
		_, vm, _, err := compileAndRunModule(t, src)
		if err != nil {
			t.Fatalf("erreur inattendue : %v", err)
		}
		exports := vm.ModuleExports()
		if len(exports) != 1 {
			t.Fatalf("nombre d'exports attendu 1, obtenu %d", len(exports))
		}
		val, ok := exports["default"]
		if !ok {
			t.Fatalf("export 'default' absent dans ModuleExports()")
		}
		s := vm.StringOf(val)
		if s == nil || s.GoString() != "ok" {
			t.Errorf("valeur export 'default' attendue 'ok', obtenu %v", val)
		}
	})
}

func TestModuleExportNamed(t *testing.T) {
	t.Run("VariablesConstLetVar", func(t *testing.T) {
		src := `
			export const x = 1, y = 2;
			export let a = "ok";
			export var z = 3;
			var total = x + y + z;
		`
		chunk, vm, _, err := compileAndRunModule(t, src)
		if err != nil {
			t.Fatalf("erreur inattendue : %v", err)
		}
		for _, name := range []string{"x", "y", "a", "z"} {
			if !chunk.HasExport(name) {
				t.Errorf("chunk.HasExport(%q) attendu true", name)
			}
		}
		vx, ok := vm.GetExport("x")
		if !ok || vx.ToInt() != 1 {
			t.Errorf("export x attendu 1, obtenu %v", vx)
		}
		vy, ok := vm.GetExport("y")
		if !ok || vy.ToInt() != 2 {
			t.Errorf("export y attendu 2, obtenu %v", vy)
		}
		va, ok := vm.GetExport("a")
		if !ok {
			t.Fatalf("export a absent")
		}
		sa := vm.StringOf(va)
		if sa == nil || sa.GoString() != "ok" {
			t.Errorf("export a attendu 'ok', obtenu %v", va)
		}
		vz, ok := vm.GetExport("z")
		if !ok || vz.ToInt() != 3 {
			t.Errorf("export z attendu 3, obtenu %v", vz)
		}
		tot, ok := vm.GetGlobal("total")
		if !ok || tot.ToInt() != 6 {
			t.Errorf("variable total attendue 6, obtenu %v", tot)
		}
	})

	t.Run("FonctionNommeeEtHoisting", func(t *testing.T) {
		src := `
			var avant = foo();
			export function foo() { return 42; }
			var apres = foo();
		`
		chunk, vm, _, err := compileAndRunModule(t, src)
		if err != nil {
			t.Fatalf("erreur inattendue : %v", err)
		}
		if !chunk.HasExport("foo") {
			t.Fatalf("chunk.HasExport(\"foo\") attendu true")
		}
		fnVal, ok := vm.GetExport("foo")
		if !ok || !vm.isFunction(fnVal) {
			t.Fatalf("export 'foo' attendu fonction")
		}
		res, err := vm.invoke(Undefined, fnVal, nil)
		if err != nil {
			t.Fatalf("échec d'invocation de foo : %v", err)
		}
		if !res.IsNumber() || res.ToInt() != 42 {
			t.Errorf("résultat invocation foo attendu 42, obtenu %v", res)
		}
		vAvant, _ := vm.GetGlobal("avant")
		if vAvant.ToInt() != 42 {
			t.Errorf("appel avant hissage attendu 42, obtenu %v", vAvant)
		}
		vApres, _ := vm.GetGlobal("apres")
		if vApres.ToInt() != 42 {
			t.Errorf("appel après déclaration attendu 42, obtenu %v", vApres)
		}
	})

	t.Run("ClasseNommee", func(t *testing.T) {
		src := `
			export class Bar {
				val() { return 100; }
			}
			var inst = new Bar();
			var res = inst.val();
		`
		chunk, vm, _, err := compileAndRunModule(t, src)
		if err != nil {
			t.Fatalf("erreur inattendue : %v", err)
		}
		if !chunk.HasExport("Bar") {
			t.Fatalf("chunk.HasExport(\"Bar\") attendu true")
		}
		clVal, ok := vm.GetExport("Bar")
		if !ok || !vm.isFunction(clVal) {
			t.Fatalf("export 'Bar' attendu constructeur")
		}
		resVal, _ := vm.GetGlobal("res")
		if resVal.ToInt() != 100 {
			t.Errorf("résultat inst.val() attendu 100, obtenu %v", resVal)
		}
	})

	t.Run("ClausesSymbolesLocaux", func(t *testing.T) {
		src := `
			var a = 10;
			var b = 20;
			export { a, b as c };
		`
		chunk, vm, _, err := compileAndRunModule(t, src)
		if err != nil {
			t.Fatalf("erreur inattendue : %v", err)
		}
		if !chunk.HasExport("a") {
			t.Errorf("chunk.HasExport(\"a\") attendu true")
		}
		if !chunk.HasExport("c") {
			t.Errorf("chunk.HasExport(\"c\") attendu true")
		}
		if chunk.HasExport("b") {
			t.Errorf("chunk.HasExport(\"b\") attendu false car renommé en c")
		}
		va, ok := vm.GetExport("a")
		if !ok || va.ToInt() != 10 {
			t.Errorf("export a attendu 10, obtenu %v", va)
		}
		vc, ok := vm.GetExport("c")
		if !ok || vc.ToInt() != 20 {
			t.Errorf("export c attendu 20, obtenu %v", vc)
		}
	})

	t.Run("ClauseSymbolesLocauxAvecFonction", func(t *testing.T) {
		src := `
			export { salut as hello };
			function salut() { return "coucou"; }
		`
		chunk, vm, _, err := compileAndRunModule(t, src)
		if err != nil {
			t.Fatalf("erreur inattendue : %v", err)
		}
		if !chunk.HasExport("hello") {
			t.Fatalf("chunk.HasExport(\"hello\") attendu true")
		}
		fnVal, ok := vm.GetExport("hello")
		if !ok || !vm.isFunction(fnVal) {
			t.Fatalf("export 'hello' attendu fonction")
		}
		res, err := vm.invoke(Undefined, fnVal, nil)
		if err != nil {
			t.Fatalf("échec d'invocation : %v", err)
		}
		s := vm.StringOf(res)
		if s == nil || s.GoString() != "coucou" {
			t.Errorf("résultat attendu 'coucou', obtenu %v", res)
		}
	})

	t.Run("DestructurationVariables", func(t *testing.T) {
		src := `export const { p1, p2 } = { p1: 11, p2: 22 };`
		chunk, vm, _, err := compileAndRunModule(t, src)
		if err != nil {
			t.Fatalf("erreur inattendue : %v", err)
		}
		if !chunk.HasExport("p1") || !chunk.HasExport("p2") {
			t.Fatalf("exports déstructurés p1 et p2 attendus")
		}
		v1, ok1 := vm.GetExport("p1")
		v2, ok2 := vm.GetExport("p2")
		if !ok1 || v1.ToInt() != 11 {
			t.Errorf("export p1 attendu 11, obtenu %v", v1)
		}
		if !ok2 || v2.ToInt() != 22 {
			t.Errorf("export p2 attendu 22, obtenu %v", v2)
		}
	})

	t.Run("VariableNonInitialisee", func(t *testing.T) {
		src := `export let uninit;`
		chunk, vm, _, err := compileAndRunModule(t, src)
		if err != nil {
			t.Fatalf("erreur inattendue : %v", err)
		}
		if !chunk.HasExport("uninit") {
			t.Fatalf("export 'uninit' attendu")
		}
		vu, ok := vm.GetExport("uninit")
		if !ok || !vu.IsUndefined() {
			t.Errorf("valeur export uninit attendue undefined, obtenu %v", vu)
		}
	})

	t.Run("RenommageEnDefault", func(t *testing.T) {
		src := `
			var answer = 42;
			export { answer as default };
		`
		chunk, vm, _, err := compileAndRunModule(t, src)
		if err != nil {
			t.Fatalf("erreur inattendue : %v", err)
		}
		if !chunk.ExportDefault {
			t.Errorf("chunk.ExportDefault attendu true")
		}
		if !chunk.HasExport("default") {
			t.Errorf("chunk.HasExport(\"default\") attendu true")
		}
		defVal, ok := vm.ModuleExportDefault()
		if !ok || defVal.ToInt() != 42 {
			t.Errorf("valeur ModuleExportDefault() attendue 42, obtenu %v", defVal)
		}
	})
}
