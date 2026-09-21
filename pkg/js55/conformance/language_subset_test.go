// SPDX-License-Identifier: Apache-2.0 OR MIT

package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const languageGolden = "testdata/manifest.language.golden"

func TestLanguageComments_JS55Engine(t *testing.T) {
	root := "/devhoros/pkg/js55/testdata/test262"
	if _, err := os.Stat(root); err != nil {
		t.Skipf("arbre test262 absent")
	}

	rep, err := RunSuite(RunnerConfig{
		Test262Root: root,
		RelPrefix:   "test/language/comments",
		Engine:      NewJS55Engine(),
		Workers:     1,
	})
	if err != nil {
		t.Fatalf("RunSuite: %v", err)
	}
	t.Log(rep.Summary())
	if rep.Stats.Pass == 0 {
		t.Fatal("aucun pass sur test/language/comments : le moteur n'est pas branché")
	}

	goldenPath := languageGolden
	goldenData, err := os.ReadFile(goldenPath)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0755); err != nil {
			t.Fatal(err)
		}
		f, err := os.Create(goldenPath)
		if err != nil {
			t.Fatal(err)
		}
		if err := WriteManifest(f, rep.Cases); err != nil {
			t.Fatal(err)
		}
		_ = f.Close()
		t.Fatalf("golden language écrit (%d cas, pass=%d) — relancer", len(rep.Cases), rep.Stats.Pass)
	} else if err != nil {
		t.Fatal(err)
	}

	goldenCases, err := ParseManifest(strings.NewReader(string(goldenData)))
	if err != nil {
		t.Fatal(err)
	}
	ratchetRep, err := VerifyRatchet(goldenCases, rep.Cases)
	if err != nil {
		t.Fatalf("ratchet: %v", err)
	}
	if ratchetRep.HasRegressions() {
		t.Errorf("%s", ratchetRep.Error())
	}
}

func TestLanguageLiterals_JS55Engine(t *testing.T) {
	runLanguagePrefix(t, "test/language/literals", "testdata/manifest.literals.golden")
}

func TestLanguageVariable_JS55Engine(t *testing.T) {
	runLanguagePrefix(t, "test/language/statements/variable", "testdata/manifest.variable.golden")
}

func TestLanguageForIn_JS55Engine(t *testing.T) {
	runLanguagePrefix(t, "test/language/statements/for-in", "testdata/manifest.for-in.golden")
}

func TestLanguageForOf_JS55Engine(t *testing.T) {
	runLanguagePrefix(t, "test/language/statements/for-of", "testdata/manifest.for-of.golden")
}

func TestLanguageCoalesce_JS55Engine(t *testing.T) {
	runLanguagePrefix(t, "test/language/expressions/coalesce", "testdata/manifest.coalesce.golden")
}

func TestLanguageOptionalChaining_JS55Engine(t *testing.T) {
	runLanguagePrefix(t, "test/language/expressions/optional-chaining", "testdata/manifest.optional-chaining.golden")
}

func TestLanguageExpressionsWave1_JS55Engine(t *testing.T) {
	dirs := []string{
		"addition", "subtraction", "multiplication", "division", "modulus", "exponentiation",
		"concatenation",
		"equals", "does-not-equals", "strict-equals", "strict-does-not-equals",
		"greater-than", "greater-than-or-equal", "less-than", "less-than-or-equal",
		"bitwise-and", "bitwise-or", "bitwise-xor", "bitwise-not",
		"left-shift", "right-shift", "unsigned-right-shift",
		"logical-and", "logical-or", "logical-not",
		"unary-minus", "unary-plus", "typeof", "void",
		"grouping", "comma", "conditional", "this",
		"in", "instanceof",
		"postfix-increment", "prefix-increment",
	}
	for _, d := range dirs {
		t.Run(d, func(t *testing.T) {
			runLanguagePrefix(t, "test/language/expressions/"+d, "testdata/manifest.expr-"+d+".golden")
		})
	}
}

func TestLanguageExpressionsWave2_JS55Engine(t *testing.T) {
	dirs := []string{
		"assignment", "compound-assignment", "object", "array",
		"call", "new", "member-expression", "property-accessors",
		"template-literal", "delete", "postfix-decrement", "prefix-decrement",
	}
	for _, d := range dirs {
		t.Run(d, func(t *testing.T) {
			runLanguagePrefix(t, "test/language/expressions/"+d, "testdata/manifest.expr-"+d+".golden")
		})
	}
}

func TestLanguageStatementsWave2_JS55Engine(t *testing.T) {
	dirs := []string{"function", "class", "const", "let"}
	for _, d := range dirs {
		t.Run(d, func(t *testing.T) {
			runLanguagePrefix(t, "test/language/statements/"+d, "testdata/manifest.stmt-"+d+".golden")
		})
	}
}

func TestBuiltinsArrayIsArray_JS55Engine(t *testing.T) {
	runLanguagePrefix(t, "test/built-ins/Array/isArray", "testdata/manifest.array-isarray.golden")
}

func TestBuiltinsArrayFrom_JS55Engine(t *testing.T) {
	runLanguagePrefix(t, "test/built-ins/Array/from", "testdata/manifest.array-from.golden")
}

func TestBuiltinsArrayOf_JS55Engine(t *testing.T) {
	runLanguagePrefix(t, "test/built-ins/Array/of", "testdata/manifest.array-of.golden")
}

func TestBuiltinsArrayPrototypeSlice_JS55Engine(t *testing.T) {
	dirs := []string{"push", "pop", "slice", "join", "concat", "toString", "shift", "unshift", "indexOf", "includes", "map", "filter", "forEach"}
	for _, d := range dirs {
		t.Run(d, func(t *testing.T) {
			runLanguagePrefix(t, "test/built-ins/Array/prototype/"+d, "testdata/manifest.array-proto-"+d+".golden")
		})
	}
}

func TestBuiltinsBoolean_JS55Engine(t *testing.T) {
	runLanguagePrefix(t, "test/built-ins/Boolean", "testdata/manifest.boolean.golden")
}

func TestBuiltinsError_JS55Engine(t *testing.T) {
	runLanguagePrefix(t, "test/built-ins/Error", "testdata/manifest.error.golden")
}

func TestBuiltinsDateNow_JS55Engine(t *testing.T) {
	runLanguagePrefix(t, "test/built-ins/Date/now", "testdata/manifest.date-now.golden")
}

func TestBuiltinsParseInt_JS55Engine(t *testing.T) {
	runLanguagePrefix(t, "test/built-ins/parseInt", "testdata/manifest.parseint.golden")
}

func TestBuiltinsParseFloat_JS55Engine(t *testing.T) {
	runLanguagePrefix(t, "test/built-ins/parseFloat", "testdata/manifest.parsefloat.golden")
}

func TestBuiltinsReflectGet_JS55Engine(t *testing.T) {
	runLanguagePrefix(t, "test/built-ins/Reflect/get", "testdata/manifest.reflect-get.golden")
}

func TestBuiltinsReflectHas_JS55Engine(t *testing.T) {
	runLanguagePrefix(t, "test/built-ins/Reflect/has", "testdata/manifest.reflect-has.golden")
}

func TestBuiltinsReflect_JS55Engine(t *testing.T) {
	runLanguagePrefix(t, "test/built-ins/Reflect", "testdata/manifest.reflect.golden")
}

func TestBuiltinsDateParse_JS55Engine(t *testing.T) {
	runLanguagePrefix(t, "test/built-ins/Date/parse", "testdata/manifest.date-parse.golden")
}

func TestBuiltinsSymbol_JS55Engine(t *testing.T) {
	runLanguagePrefix(t, "test/built-ins/Symbol", "testdata/manifest.symbol.golden")
}

func TestBuiltinsPromise_JS55Engine(t *testing.T) {
	runLanguagePrefix(t, "test/built-ins/Promise", "testdata/manifest.promise.golden")
}

func TestBuiltinsProxy_JS55Engine(t *testing.T) {
	runLanguagePrefix(t, "test/built-ins/Proxy", "testdata/manifest.proxy.golden")
}

func TestBuiltinsRegExp_JS55Engine(t *testing.T) {
	runLanguagePrefix(t, "test/built-ins/RegExp", "testdata/manifest.regexp.golden")
}

func TestBuiltinsRegExpPrototypeTest_JS55Engine(t *testing.T) {
	runLanguagePrefix(t, "test/built-ins/RegExp/prototype/test", "testdata/manifest.regexp-proto-test.golden")
}

func TestBuiltinsRegExpPrototypeExec_JS55Engine(t *testing.T) {
	runLanguagePrefix(t, "test/built-ins/RegExp/prototype/exec", "testdata/manifest.regexp-proto-exec.golden")
}

func TestLanguageExpressionsYield_JS55Engine(t *testing.T) {
	runLanguagePrefix(t, "test/language/expressions/yield", "testdata/manifest.expr-yield.golden")
}

func TestLanguageStatementsGenerators_JS55Engine(t *testing.T) {
	runLanguagePrefix(t, "test/language/statements/generators", "testdata/manifest.stmt-generators.golden")
}

func TestLanguageExpressionsGenerators_JS55Engine(t *testing.T) {
	runLanguagePrefix(t, "test/language/expressions/generators", "testdata/manifest.expr-generators.golden")
}

func TestBuiltinsGeneratorPrototypeNext_JS55Engine(t *testing.T) {
	runLanguagePrefix(t, "test/built-ins/GeneratorPrototype/next", "testdata/manifest.genproto-next.golden")
}

func TestBuiltinsGeneratorPrototypeReturn_JS55Engine(t *testing.T) {
	runLanguagePrefix(t, "test/built-ins/GeneratorPrototype/return", "testdata/manifest.genproto-return.golden")
}

func TestLanguageStatementsForOf_JS55Engine(t *testing.T) {
	runLanguagePrefix(t, "test/language/statements/for-of", "testdata/manifest.stmt-for-of.golden")
}

func TestLanguageStatementsForAwaitOf_JS55Engine(t *testing.T) {
	runLanguagePrefix(t, "test/language/statements/for-await-of", "testdata/manifest.stmt-for-await-of.golden")
}

func TestLanguageExpressionsTaggedTemplate_JS55Engine(t *testing.T) {
	runLanguagePrefix(t, "test/language/expressions/tagged-template", "testdata/manifest.expr-tagged-template.golden")
}

func TestBuiltinsGeneratorPrototypeThrow_JS55Engine(t *testing.T) {
	runLanguagePrefix(t, "test/built-ins/GeneratorPrototype/throw", "testdata/manifest.genproto-throw.golden")
}

func TestLanguageStatementsWave1_JS55Engine(t *testing.T) {
	dirs := []string{
		"if", "while", "do-while", "for", "block",
		"break", "continue", "return", "switch",
		"throw", "try", "empty",
	}
	for _, d := range dirs {
		t.Run(d, func(t *testing.T) {
			runLanguagePrefix(t, "test/language/statements/"+d, "testdata/manifest.stmt-"+d+".golden")
		})
	}
}

func TestBuiltinsMath_JS55Engine(t *testing.T) {
	runLanguagePrefix(t, "test/built-ins/Math", "testdata/manifest.math.golden")
}

func TestBuiltinsJSON_JS55Engine(t *testing.T) {
	runLanguagePrefix(t, "test/built-ins/JSON", "testdata/manifest.json.golden")
}

func TestBuiltinsNumber_JS55Engine(t *testing.T) {
	runLanguagePrefix(t, "test/built-ins/Number", "testdata/manifest.number.golden")
}

func TestBuiltinsObject_JS55Engine(t *testing.T) {
	runLanguagePrefix(t, "test/built-ins/Object", "testdata/manifest.object.golden")
}

func TestLanguage_Object(t *testing.T) {
	runLanguagePrefix(t, "test/built-ins/Object", "testdata/manifest.object.golden")
}

func TestBuiltinsObjectDefineProperty_JS55Engine(t *testing.T) {
	runLanguagePrefix(t, "test/built-ins/Object/defineProperty", "testdata/manifest.object-defineproperty.golden")
}

func TestBuiltinsArray_JS55Engine(t *testing.T) {
	t.Skip("test/built-ins/Array (~3082 fichiers) sature la RAM à la génération du golden — skip nommé, pas retiré")
	runLanguagePrefix(t, "test/built-ins/Array", "testdata/manifest.array.golden")
}

func TestBuiltinsFunction_JS55Engine(t *testing.T) {
	runLanguagePrefix(t, "test/built-ins/Function", "testdata/manifest.function.golden")
}

func TestBuiltinsString_JS55Engine(t *testing.T) {
	runLanguagePrefix(t, "test/built-ins/String", "testdata/manifest.string.golden")
}

func TestBuiltinsMap_JS55Engine(t *testing.T) {
	runLanguagePrefix(t, "test/built-ins/Map", "testdata/manifest.map.golden")
}

func TestBuiltinsSet_JS55Engine(t *testing.T) {
	runLanguagePrefix(t, "test/built-ins/Set", "testdata/manifest.set.golden")
}

func runLanguagePrefix(t *testing.T, prefix, goldenPath string) {
	t.Helper()
	root := "/devhoros/pkg/js55/testdata/test262"
	if _, err := os.Stat(root); err != nil {
		t.Skipf("arbre test262 absent")
	}
	rep, err := RunSuite(RunnerConfig{
		Test262Root: root,
		RelPrefix:   prefix,
		Engine:      NewJS55Engine(),
		Workers:     0,
	})
	if err != nil {
		t.Fatalf("RunSuite: %v", err)
	}
	t.Log(rep.Summary())
	if rep.Stats.Pass == 0 {
		t.Fatalf("aucun pass sur %s", prefix)
	}
	goldenData, err := os.ReadFile(goldenPath)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0755); err != nil {
			t.Fatal(err)
		}
		f, err := os.Create(goldenPath)
		if err != nil {
			t.Fatal(err)
		}
		if err := WriteManifest(f, rep.Cases); err != nil {
			t.Fatal(err)
		}
		_ = f.Close()
		t.Fatalf("golden %s écrit (%d cas, pass=%d) — relancer", goldenPath, len(rep.Cases), rep.Stats.Pass)
	} else if err != nil {
		t.Fatal(err)
	}
	goldenCases, err := ParseManifest(strings.NewReader(string(goldenData)))
	if err != nil {
		t.Fatal(err)
	}
	ratchetRep, err := VerifyRatchet(goldenCases, rep.Cases)
	if err != nil {
		t.Fatalf("ratchet: %v", err)
	}
	if os.Getenv("JS55_WRITE_GOLDEN") == "1" {
		f, err := os.Create(goldenPath)
		if err != nil {
			t.Fatal(err)
		}
		if err := WriteManifest(f, rep.Cases); err != nil {
			_ = f.Close()
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		t.Logf("golden %s réécrit (cas=%d, pass=%d)", goldenPath, len(rep.Cases), rep.Stats.Pass)
	}
	if ratchetRep.HasRegressions() {
		t.Errorf("%s", ratchetRep.Error())
	}
}

func TestLanguage_NamedPass_S7_4_A1_T1(t *testing.T) {
	c := lookupLanguageCase(t, "test/language/comments/S7.4_A1_T1.js", ModeSloppy)
	if c.Verdict != VerdictPass {
		t.Fatalf("S7.4_A1_T1 sloppy: %s (%s)", c.Verdict, c.Error)
	}
}

func TestLanguage_NamedPass_HashbangNotEmpty(t *testing.T) {
	c := lookupLanguageCase(t, "test/language/comments/hashbang/not-empty.js", ModeSloppy)
	if c.Verdict != VerdictPass {
		t.Fatalf("hashbang/not-empty.js sloppy (positif raw): %s (%s)", c.Verdict, c.Error)
	}
}

func TestLanguage_NamedReject_S7_4_A3(t *testing.T) {
	c := lookupLanguageCase(t, "test/language/comments/S7.4_A3.js", ModeSloppy)
	if c.Verdict != VerdictPass {
		t.Fatalf("S7.4_A3 (négatif parse SyntaxError) sloppy: %s (%s)", c.Verdict, c.Error)
	}
}

func lookupLanguageCase(t *testing.T, rel string, mode Mode) TestCase {
	t.Helper()
	root := "/devhoros/pkg/js55/testdata/test262"
	if _, err := os.Stat(root); err != nil {
		t.Skipf("arbre test262 absent")
	}
	rep, err := RunSuite(RunnerConfig{
		Test262Root: root,
		RelPrefix:   "test/language/comments",
		Engine:      NewJS55Engine(),
		Workers:     1,
	})
	if err != nil {
		t.Fatalf("RunSuite: %v", err)
	}
	for _, c := range rep.Cases {
		if c.RelPath == rel && c.Mode == mode {
			return c
		}
	}
	t.Fatalf("cas absent: %s %s", rel, mode)
	return TestCase{}
}

func TestObject_LodashFragment_CompileAndRun(t *testing.T) {
	eng := NewJS55Engine()
	src := `
		function has(obj, key) {
			return obj != null && Object.prototype.hasOwnProperty.call(obj, key);
		}
		function assignValue(object, key, value) {
			var objValue = object[key];
			if (!Object.prototype.hasOwnProperty.call(object, key) || !Object.is(objValue, value)) {
				object[key] = value;
			}
		}
		function copyObject(source, props, object) {
			var isNew = !object;
			object || (object = {});
			var index = -1, length = props.length;
			while (++index < length) {
				var key = props[index];
				var newValue = source[key];
				assignValue(object, key, newValue);
			}
			return object;
		}
		var src = { a: 1, b: 2, c: 3 };
		var res = copyObject(src, Object.keys(src));
		if (res.a !== 1 || res.b !== 2 || res.c !== 3) {
			throw new Error("fail copyObject");
		}
		if (!has(res, "a") || !has(res, "b") || !has(res, "c")) {
			throw new Error("fail has");
		}
	`
	if err := eng.Eval(src, false); err != nil {
		t.Fatalf("Lodash fragment failure: %v", err)
	}
}

func TestObject_ZodFragment_CompileAndRun(t *testing.T) {
	eng := NewJS55Engine()
	src := `
		var ZodParsedType = {
			object: "object",
			string: "string",
			number: "number",
			boolean: "boolean",
			undefined: "undefined",
			null: "null"
		};
		function getParsedType(data) {
			if (data === null) return ZodParsedType.null;
			if (data === undefined) return ZodParsedType.undefined;
			var t = typeof data;
			if (t === "object") return ZodParsedType.object;
			if (t === "string") return ZodParsedType.string;
			if (t === "number") return ZodParsedType.number;
			if (t === "boolean") return ZodParsedType.boolean;
			return "other";
		}
		function ZodObject(shape) {
			this._shape = shape;
		}
		ZodObject.prototype.parse = function(input) {
			if (getParsedType(input) !== ZodParsedType.object) {
				throw new Error("Expected object, received " + getParsedType(input));
			}
			var result = {};
			var keys = Object.keys(this._shape);
			for (var i = 0; i < keys.length; i++) {
				var k = keys[i];
				var validator = this._shape[k];
				var val = input[k];
				if (typeof validator === "function") {
					result[k] = validator(val);
				} else {
					result[k] = val;
				}
			}
			return result;
		};
		var schema = new ZodObject({
			name: function(v) {
				if (typeof v !== "string") throw new Error("Expected string");
				return v;
			},
			age: function(v) {
				if (typeof v !== "number") throw new Error("Expected number");
				return v;
			}
		});
		var parsed = schema.parse({ name: "alice", age: 30 });
		if (parsed.name !== "alice" || parsed.age !== 30) {
			throw new Error("failed schema parsing");
		}
	`
	if err := eng.Eval(src, false); err != nil {
		t.Fatalf("Zod fragment failure: %v", err)
	}
}
