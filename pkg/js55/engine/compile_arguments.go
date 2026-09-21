// SPDX-License-Identifier: BUSL-1.1
package engine

import (
	"reflect"

	"github.com/hazyhaar/js55/pkg/js55/ast"
)

func bodyNeedsArguments(body *ast.BlockStmt) bool {
	return nodeNeedsArguments(body)
}

func nodeNeedsArguments(n ast.Node) bool {
	if n == nil {
		return false
	}
	if rv := reflect.ValueOf(n); rv.Kind() == reflect.Pointer && rv.IsNil() {
		return false
	}
	switch t := n.(type) {
	case *ast.FunctionExpr, *ast.FunctionDecl:
		return false
	case *ast.ArrowFunction:
		return nodeNeedsArguments(t.Body) || exprsNeedArguments(t.Params)
	case *ast.ClassExpr, *ast.ClassDecl:
		return true
	case *ast.Ident:
		return t.Name == "arguments"
	case *ast.CallExpr:
		if id, ok := t.Callee.(*ast.Ident); ok && id != nil && id.Name == "eval" {
			return true
		}
		if nodeNeedsArguments(t.Callee) {
			return true
		}
		return exprsNeedArguments(t.Args)
	case *ast.NewExpr:
		if nodeNeedsArguments(t.Callee) {
			return true
		}
		return exprsNeedArguments(t.Args)
	case *ast.MemberExpr:
		if nodeNeedsArguments(t.Object) {
			return true
		}
		if t.Computed {
			return nodeNeedsArguments(t.Property)
		}
		return false
	case *ast.BlockStmt:
		return stmtsNeedArguments(t.Body)
	case *ast.ExpressionStmt:
		return nodeNeedsArguments(t.Expression)
	case *ast.ReturnStmt:
		return nodeNeedsArguments(t.Argument)
	case *ast.ThrowStmt:
		return nodeNeedsArguments(t.Argument)
	case *ast.IfStmt:
		return nodeNeedsArguments(t.Test) || nodeNeedsArguments(t.Consequent) || nodeNeedsArguments(t.Alternate)
	case *ast.ForStmt:
		return nodeNeedsArguments(t.Init) || nodeNeedsArguments(t.Test) || nodeNeedsArguments(t.Update) || nodeNeedsArguments(t.Body)
	case *ast.ForInStmt:
		return nodeNeedsArguments(t.Left) || nodeNeedsArguments(t.Right) || nodeNeedsArguments(t.Body)
	case *ast.WhileStmt:
		return nodeNeedsArguments(t.Test) || nodeNeedsArguments(t.Body)
	case *ast.DoWhileStmt:
		return nodeNeedsArguments(t.Body) || nodeNeedsArguments(t.Test)
	case *ast.LabeledStmt:
		return nodeNeedsArguments(t.Body)
	case *ast.TryStmt:
		if nodeNeedsArguments(t.Block) || nodeNeedsArguments(t.Finalizer) {
			return true
		}
		if t.Handler != nil {
			return nodeNeedsArguments(t.Handler.Param) || nodeNeedsArguments(t.Handler.Body)
		}
		return false
	case *ast.SwitchStmt:
		if nodeNeedsArguments(t.Discriminant) {
			return true
		}
		for _, c := range t.Cases {
			if c == nil {
				continue
			}
			if nodeNeedsArguments(c.Test) || stmtsNeedArguments(c.Body) {
				return true
			}
		}
		return false
	case *ast.WithStmt:
		return nodeNeedsArguments(t.Object) || nodeNeedsArguments(t.Body)
	case *ast.VarDecl:
		for _, d := range t.Decls {
			if d != nil && (nodeNeedsArguments(d.Target) || nodeNeedsArguments(d.Init)) {
				return true
			}
		}
		return false
	case *ast.UnaryExpr, *ast.UpdateExpr:
		switch u := n.(type) {
		case *ast.UnaryExpr:
			return nodeNeedsArguments(u.Operand)
		case *ast.UpdateExpr:
			return nodeNeedsArguments(u.Operand)
		}
		return false
	case *ast.BinaryExpr:
		return nodeNeedsArguments(t.Left) || nodeNeedsArguments(t.Right)
	case *ast.LogicalExpr:
		return nodeNeedsArguments(t.Left) || nodeNeedsArguments(t.Right)
	case *ast.AssignExpr:
		return nodeNeedsArguments(t.Target) || nodeNeedsArguments(t.Value)
	case *ast.ConditionalExpr:
		return nodeNeedsArguments(t.Test) || nodeNeedsArguments(t.Consequent) || nodeNeedsArguments(t.Alternate)
	case *ast.SequenceExpr:
		return exprsNeedArguments(t.Exprs)
	case *ast.SpreadElement:
		return nodeNeedsArguments(t.Argument)
	case *ast.YieldExpr:
		return nodeNeedsArguments(t.Argument)
	case *ast.AwaitExpr:
		return nodeNeedsArguments(t.Argument)
	case *ast.ChainExpr:
		return nodeNeedsArguments(t.Expression)
	case *ast.ArrayLit:
		return exprsNeedArguments(t.Elements)
	case *ast.ObjectLit:
		for _, p := range t.Properties {
			if p == nil {
				continue
			}
			if p.Computed && nodeNeedsArguments(p.Key) {
				return true
			}
			if nodeNeedsArguments(p.Value) {
				return true
			}
		}
		return false
	case *ast.ArrayPattern:
		return exprsNeedArguments(t.Elements)
	case *ast.ObjectPattern:
		for _, p := range t.Properties {
			if p != nil && (nodeNeedsArguments(p.Key) || nodeNeedsArguments(p.Value)) {
				return true
			}
		}
		return false
	case *ast.AssignPattern:
		return nodeNeedsArguments(t.Target) || nodeNeedsArguments(t.Default)
	case *ast.RestElement:
		return nodeNeedsArguments(t.Argument)
	case *ast.TemplateLit:
		return exprsNeedArguments(t.Exprs)
	case *ast.TaggedTemplate:
		return nodeNeedsArguments(t.Tag) || nodeNeedsArguments(t.Quasi)
	case *ast.ThisExpr, *ast.SuperExpr, *ast.EmptyStmt, *ast.DebuggerStmt, *ast.BreakStmt, *ast.ContinueStmt:
		return false
	case *ast.NumberLit, *ast.BigIntLit, *ast.StringLit, *ast.BoolLit, *ast.NullLit, *ast.RegExpLit, *ast.PrivateName, *ast.MetaProperty:
		return false
	default:
		return true
	}
}

func exprsNeedArguments(exprs []ast.Expr) bool {
	for _, e := range exprs {
		if nodeNeedsArguments(e) {
			return true
		}
	}
	return false
}

func stmtsNeedArguments(stmts []ast.Stmt) bool {
	for _, s := range stmts {
		if nodeNeedsArguments(s) {
			return true
		}
	}
	return false
}
