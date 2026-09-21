// SPDX-License-Identifier: BUSL-1.1
package parser_test

import (
	"context"
	"strings"
	"testing"

	"github.com/hazyhaar/js55/pkg/js55/ast"
	"github.com/hazyhaar/js55/pkg/js55/isolate"
	"github.com/hazyhaar/js55/pkg/js55/parser"
)

// TestParser_TypeScript_VariableAnnotations teste l'effacement des annotations sur les variables.
func TestParser_TypeScript_VariableAnnotations(t *testing.T) {
	src := `let a: number = 10; const s: string = "hello"; var arr: Array<number> = [1, 2];`
	prog, err := parser.Parse(src, parser.Options{TypeScript: true})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(prog.Body) != 3 {
		t.Fatalf("attendu 3 statements, obtenu %d", len(prog.Body))
	}
	v1, ok := prog.Body[0].(*ast.VarDecl)
	if !ok || len(v1.Decls) != 1 {
		t.Fatalf("déclaration 1 invalide: %#v", prog.Body[0])
	}
	ident, ok := v1.Decls[0].Target.(*ast.Ident)
	if !ok || ident.Name != "a" {
		t.Fatalf("nom attendu 'a', obtenu: %#v", v1.Decls[0].Target)
	}
}

// TestParser_TypeScript_FunctionSignatures teste les fonctions et flèches avec types et retours.
func TestParser_TypeScript_FunctionSignatures(t *testing.T) {
	src := `
		function add(x: number, y: number): number { return x + y; }
		function greet(name: string, title?: string): string { return name; }
		const mul = (a: number, b: number): number => a * b;
	`
	prog, err := parser.Parse(src, parser.Options{TypeScript: true})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(prog.Body) != 3 {
		t.Fatalf("attendu 3 statements, obtenu %d", len(prog.Body))
	}
	fn1, ok := prog.Body[0].(*ast.FunctionDecl)
	if !ok || len(fn1.Fn.Params) != 2 {
		t.Fatalf("fn add invalide: %#v", prog.Body[0])
	}
}

// TestParser_TypeScript_InterfacesAndTypes teste l'effacement complet des interfaces et types.
func TestParser_TypeScript_InterfacesAndTypes(t *testing.T) {
	src := `
		interface User<T> {
			id: number;
			data: T;
		}
		type ID = string | number;
		const uid: ID = 42;
	`
	prog, err := parser.Parse(src, parser.Options{TypeScript: true})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	// interface et type génèrent des EmptyStmt, puis la constante
	hasVar := false
	for _, stmt := range prog.Body {
		if v, ok := stmt.(*ast.VarDecl); ok && len(v.Decls) > 0 {
			if id, ok := v.Decls[0].Target.(*ast.Ident); ok && id.Name == "uid" {
				hasVar = true
			}
		}
	}
	if !hasVar {
		t.Fatal("variable 'uid' introuvable dans l'AST")
	}
}

// TestParser_TypeScript_AsAndNonNull teste les assertions 'as Type' et 'expr!'.
func TestParser_TypeScript_AsAndNonNull(t *testing.T) {
	src := `
		const x = (obj as any).value!;
		const y = val as string;
	`
	prog, err := parser.Parse(src, parser.Options{TypeScript: true})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(prog.Body) != 2 {
		t.Fatalf("attendu 2 statements, obtenu %d", len(prog.Body))
	}
}

// TestParser_TypeScript_ClassModifiers teste les modificateurs de classe (public, private, etc.).
func TestParser_TypeScript_ClassModifiers(t *testing.T) {
	src := `
		class Point {
			public x: number = 0;
			private y: number = 0;
			protected readonly z: number = 10;
			override toString(): string {
				return "point";
			}
		}
	`
	prog, err := parser.Parse(src, parser.Options{TypeScript: true})
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if len(prog.Body) != 1 {
		t.Fatalf("attendu 1 statement, obtenu %d", len(prog.Body))
	}
	cl, ok := prog.Body[0].(*ast.ClassDecl)
	if !ok || len(cl.Class.Body) != 4 {
		t.Fatalf("classe invalide: %#v", prog.Body[0])
	}
}

// TestParser_TypeScript_EnumRejection vérifie le rejet propre et explicite des enum et namespace.
func TestParser_TypeScript_EnumRejection(t *testing.T) {
	cases := []string{
		`enum Direction { Up, Down }`,
		`namespace MathUtils { export function add() {} }`,
	}
	for _, c := range cases {
		_, err := parser.Parse(c, parser.Options{TypeScript: true})
		if err == nil {
			t.Fatalf("attendu erreur de syntaxe pour %q, obtenu nil", c)
		}
		if sErr, ok := err.(*parser.SyntaxError); ok {
			expected := "les enums/namespaces TypeScript avec génération de code au runtime ne sont pas supportés"
			if !strings.Contains(sErr.Msg, expected) {
				t.Fatalf("message attendu contenant %q, obtenu %q", expected, sErr.Msg)
			}
		}
	}
}

// TestIsolate_TypeScript_EndToEnd exécute un script complet TypeScript dans un Isolate.
func TestIsolate_TypeScript_EndToEnd(t *testing.T) {
	iso, err := isolate.New(isolate.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer iso.Close()

	tsScript := `
		interface Vector {
			x: number;
			y: number;
		}
		type Numeric = number;

		function dot(a: Vector, b: Vector): Numeric {
			return a.x * b.x + a.y * b.y;
		}

		class Calculator {
			public offset: number;
			constructor(initialOffset: number) {
				this.offset = initialOffset;
			}
			compute(v1: Vector, v2: Vector): number {
				const res: number = dot(v1, v2) + this.offset;
				return res as number;
			}
		}

		const calc = new Calculator(10);
		const v1: Vector = { x: 2, y: 3 };
		const v2: Vector = { x: 4, y: 5 };
		calc.compute(v1, v2);
	`

	// Compiler avec TypeScript: true
	chunk, err := iso.Compile(tsScript, "test.ts", false)
	if err != nil {
		t.Fatalf("Échec compilation TypeScript : %v", err)
	}

	val, err := iso.Execute(context.Background(), chunk)
	if err != nil {
		t.Fatalf("Échec exécution TypeScript : %v", err)
	}

	// 2*4 + 3*5 = 8 + 15 = 23 + offset(10) = 33
	if val.ToInt() != 33 {
		t.Fatalf("attendu 33, obtenu: %v", val)
	}
	t.Logf("Exécution TypeScript End-to-End validée avec succès : résultat = %v", val)
}

// TestParser_TypeScript_GenericCalls vérifie que id<string>("a") et new Box<number>(1)
// sont bien traités comme des appels et instanciations et non des comparaisons.
func TestParser_TypeScript_GenericCalls(t *testing.T) {
	iso, err := isolate.New(isolate.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer iso.Close()

	src := `
		function id<T>(x: T): T { return x; }
		class Box<T> {
			public val: T;
			constructor(v: T) { this.val = v; }
		}
		const a = id<string>("hello");
		const b = new Box<number>(42);
		a === "hello" && b.val === 42;
	`
	chunk, err := iso.Compile(src, "generic.ts", false)
	if err != nil {
		t.Fatalf("compilation generic: %v", err)
	}
	v, err := iso.Execute(context.Background(), chunk)
	if err != nil {
		t.Fatalf("exécution generic: %v", err)
	}
	if iso.VM().ToStringValue(v).GoString() != "true" {
		t.Fatalf("attendu true, obtenu %v", v)
	}
}

// TestParser_TypeScript_ParenthesizedTernary vérifie qu'un ternaire parenthésé ne régresse pas.
func TestParser_TypeScript_ParenthesizedTernary(t *testing.T) {
	src := `const res = a ? (a) : 0;`
	_, err := parser.Parse(src, parser.Options{TypeScript: true})
	if err != nil {
		t.Fatalf("régression ternaire parenthésé: %v", err)
	}
}

// TestParser_TypeScript_ImportExportType vérifie que import type et export type/interface
// sont effacés en EmptyStmt sans générer d'import réel au runtime.
func TestParser_TypeScript_ImportExportType(t *testing.T) {
	src := `
		import type { User } from "./user";
		import type Def from "./def";
		export type ID = string | number;
		export interface Greeter { greet(): string; }
		const x = 1;
	`
	prog, err := parser.Parse(src, parser.Options{TypeScript: true, Module: true})
	if err != nil {
		t.Fatalf("import/export type: %v", err)
	}
	for _, stmt := range prog.Body {
		if _, ok := stmt.(*ast.ImportDecl); ok {
			t.Fatal("ImportDecl fictif généré pour import type !")
		}
		if exp, ok := stmt.(*ast.ExportDecl); ok && !exp.Default {
			if _, empty := exp.Declaration.(*ast.EmptyStmt); empty {
				t.Fatal("ExportDecl enveloppant EmptyStmt généré pour export type/interface !")
			}
		}
	}
}

// TestParser_TypeScript_TypedDefaultsAndDefiniteAssignment teste les paramètres typés avec valeur par défaut et let x!: number.
func TestParser_TypeScript_TypedDefaultsAndDefiniteAssignment(t *testing.T) {
	iso, err := isolate.New(isolate.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer iso.Close()

	src := `
		let x!: number;
		x = 100;
		function greet(name: string = "world", mult: number = 2): string {
			return name + " " + (x * mult);
		}
		const arrow = (a: number = 10, b: number = 20): number => a + b;
		greet() === "world 200" && arrow() === 30;
	`
	chunk, err := iso.Compile(src, "defaults.ts", false)
	if err != nil {
		t.Fatalf("Compilation typed defaults: %v", err)
	}
	v, err := iso.Execute(context.Background(), chunk)
	if err != nil {
		t.Fatalf("Exécution typed defaults: %v", err)
	}
	if !v.ToBool() {
		t.Fatalf("Échec évaluation typed defaults: got %v", v)
	}
}

// TestParser_TypeScript_AstraAuditCases vérifie les 5 cas limites audités par GPT-6 Astra.
func TestParser_TypeScript_AstraAuditCases(t *testing.T) {
	iso, err := isolate.New(isolate.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer iso.Close()

	cases := []struct {
		name     string
		src      string
		expected int64
		isBool   bool
		expBool  bool
		wantErr  bool
	}{
		{
			name:     "TypeAliasASI",
			src:      "type T = number\nconst x = 42; x;",
			expected: 42,
		},
		{
			name:     "NestedGenericsShr",
			src:      "const x: Array<Array<number>> = [[42]]; x[0][0];",
			expected: 42,
		},
		{
			name:    "AsTypeLessThanComparison",
			src:     "const x = 1 as number < 2; x;",
			isBool:  true,
			expBool: true,
		},
		{
			name:     "AsTypeNullishCoalescing",
			src:      "const x = null as any ?? 42; x;",
			expected: 42,
		},
		{
			name:    "UnclosedInterfaceError",
			src:     "interface Broken {",
			wantErr: true,
		},
		{
			name:    "AsTypeChainedComparison",
			src:     "const x = 1 as number < 2 > 0; x;",
			isBool:  true,
			expBool: true,
		},
		{
			name:     "AsTypeTernary",
			src:      "const x = 0 as number ? 10 : 20; x;",
			expected: 20,
		},
		{
			name:     "AsTypeBitwiseAnd",
			src:      "const x = 3 as number & 1; x;",
			expected: 3,
		},
		{
			name:     "AsTypeBitwiseOr",
			src:      "const x = 1 as number | 2; x;",
			expected: 1,
		},
		{
			name:     "AsTypeUnionString",
			src:      "const x = 3 as number | string; x;",
			expected: 3,
		},
		{
			name:     "AsTypeIntersectionObject",
			src:      "const x = 3 as number & {}; x;",
			expected: 3,
		},
		{
			name:    "MissingTypeAfterAs",
			src:     "const x = 42 as",
			wantErr: true,
		},
		{
			name:     "TypeUnionNewlineContinuation",
			src:      "type T = number |\n string; const x = 42; x;",
			expected: 42,
		},
		{
			name:     "NestedGenericsNoSpaceAssign",
			src:      "const x: Array<Array<number>>=[[42]]; x[0][0];",
			expected: 42,
		},
		{
			name:    "UnclosedGenericTypeInAlias",
			src:     "type T = Array<number",
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chunk, err := iso.Compile(tc.src, tc.name+".ts", false)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("%s: erreur de compilation attendue, obtenu nil", tc.name)
				}
				return
			}
			if err != nil {
				t.Fatalf("%s: erreur de compilation inattendue: %v", tc.name, err)
			}
			v, runErr := iso.Execute(context.Background(), chunk)
			if runErr != nil {
				t.Fatalf("%s: erreur d'exécution inattendue: %v", tc.name, runErr)
			}
			if tc.isBool {
				if v.ToBool() != tc.expBool {
					t.Fatalf("%s: attendu bool %v, obtenu %v", tc.name, tc.expBool, v.ToBool())
				}
			} else {
				if int64(v.ToInt()) != tc.expected {
					t.Fatalf("%s: attendu %d, obtenu %d (%s)", tc.name, tc.expected, v.ToInt(), v.String())
				}
			}
		})
	}
}
