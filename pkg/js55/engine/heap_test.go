package engine

import (
	"fmt"
	"math/rand"
	"runtime"
	"testing"

	"github.com/hazyhaar/js55/pkg/js55/str"
)

// Instruments T3.1 à T3.5 du plan. Le ramasse-miettes se teste EN PREMIER :
// c'est le poste où les moteurs meurent, et le mode stress est le seul
// instrument qui trouve les défauts d'enracinement.

func k(h *Heap, name string) *str.String { return h.Intern().InternGo(name) }

// ─── Formes ─────────────────────────────────────────────────────────────────

func TestShapesConverge(t *testing.T) {
	h := NewHeap()

	// Deux objets ayant reçu les mêmes clés dans le même ordre doivent partager
	// la MÊME forme. Sans cela, le modèle ne sert à rien.
	a := h.NewObject()
	b := h.NewObject()
	for _, name := range []string{"x", "y", "z"} {
		h.SetProperty(a, k(h, name), Int(1))
		h.SetProperty(b, k(h, name), Int(2))
	}
	if h.MustGet(a).Shape() != h.MustGet(b).Shape() {
		t.Error("deux objets construits identiquement ne partagent pas leur forme")
	}

	// Un ordre différent donne une forme différente : c'est voulu, l'ordre des
	// clés est observable en JavaScript.
	c := h.NewObject()
	for _, name := range []string{"y", "x", "z"} {
		h.SetProperty(c, k(h, name), Int(3))
	}
	if h.MustGet(c).Shape() == h.MustGet(a).Shape() {
		t.Error("un ordre d'insertion différent donne la même forme")
	}

	// La clé est comparée par IDENTITÉ de pointeur, ce qui exige l'internement.
	// Une clé construite autrement doit néanmoins résoudre.
	got, ok := h.GetOwnProperty(a, str.FromGo("y"))
	if !ok || got.ToInt() != 1 {
		t.Errorf("résolution par clé non internée : %v, %v", got, ok)
	}

	// Et une clé de FORME différente doit résoudre aussi — c'est ce que
	// l'internement indépendant de la forme garantit (jalon 2).
	saved := str.ForceUTF16()
	str.SetForceUTF16(true)
	got, ok = h.GetOwnProperty(a, str.FromGo("y"))
	str.SetForceUTF16(saved)
	if !ok || got.ToInt() != 1 {
		t.Errorf("résolution par clé de forme large : %v, %v", got, ok)
	}
}

func TestPrototypeChain(t *testing.T) {
	h := NewHeap()
	base := h.NewObject()
	h.SetProperty(base, k(h, "hérité"), Int(7))

	derived := h.NewObject()
	h.SetProto(derived, base)

	if v, ok := h.GetProperty(derived, k(h, "hérité")); !ok || v.ToInt() != 7 {
		t.Errorf("remontée de prototype : %v, %v", v, ok)
	}
	if _, ok := h.GetOwnProperty(derived, k(h, "hérité")); ok {
		t.Error("la propriété héritée est rendue comme propre")
	}

	// Un cycle de prototypes doit rendre Undefined, jamais boucler.
	h.SetProto(base, derived)
	done := make(chan bool, 1)
	go func() {
		_, ok := h.GetProperty(derived, k(h, "absente"))
		done <- ok
	}()
	select {
	case ok := <-done:
		if ok {
			t.Error("une propriété absente est trouvée dans un cycle")
		}
	case <-make(chan struct{}):
	}
}

// ─── T3.1 : mode stress ─────────────────────────────────────────────────────

// buildGraph construit un graphe d'objets enraciné et rend la racine ainsi que
// la somme attendue des feuilles.
func buildGraph(h *Heap, root *Value, depth, width int) int {
	sum := 0
	next := 1

	var build func(d int) Handle
	build = func(d int) Handle {
		obj := h.NewObject()
		// L'objet en construction doit être enraciné pendant que ses enfants
		// s'allouent : en mode stress, chaque allocation collecte, et un objet
		// non enraciné disparaît sous les pieds de son constructeur.
		saved := *root
		h.Push(ObjectValue(obj))
		defer func() { h.Pop(); *root = saved }()

		if d == 0 {
			h.SetProperty(obj, k(h, "valeur"), Int(int32(next)))
			sum += next
			next++
			return obj
		}
		for i := 0; i < width; i++ {
			child := build(d - 1)
			h.SetProperty(obj, k(h, fmt.Sprintf("e%d", i)), ObjectValue(child))
		}
		return obj
	}

	*root = ObjectValue(build(depth))
	return sum
}

func walkSum(h *Heap, v Value) int {
	if !v.IsObject() {
		return 0
	}
	o := h.Get(v.Handle())
	if o == nil {
		return -1 << 30 // objet libéré : la somme deviendra manifestement fausse
	}
	if leaf, ok := h.GetOwnProperty(v.Handle(), k(h, "valeur")); ok {
		return int(leaf.ToInt())
	}
	total := 0
	for _, key := range o.Shape().Keys() {
		child, _ := h.GetOwnProperty(v.Handle(), key)
		total += walkSum(h, child)
	}
	return total
}

// TestT3_1_StressGC exige un résultat IDENTIQUE en mode normal et en mode
// stress. Un écart entre les deux n'est pas une tolérance : c'est un défaut
// d'enracinement, et cet unique instrument attrape toute la classe.
func TestT3_1_StressGC(t *testing.T) {
	for _, stress := range []bool{false, true} {
		h := NewHeap()
		h.SetStress(stress)

		var root Value = Undefined
		h.AddRoot(&root)

		want := buildGraph(h, &root, 4, 3)
		got := walkSum(h, root)

		mode := "normal"
		if stress {
			mode = "stress"
		}
		if got != want {
			t.Errorf("mode %s : somme %d, %d attendu — un objet a été collecté "+
				"alors qu'il était encore atteignable", mode, got, want)
		}
		if err := h.CheckHeap(); err != nil {
			t.Errorf("mode %s : invariant de tas rompu : %v", mode, err)
		}
		t.Logf("mode %-7s : %d collectes, %d objets vivants, %d balayés",
			mode, h.Collections, h.Live(), h.Swept)
	}
}

// TestT3_1_StressFindsMissingRoot prouve que le mode stress mesure quelque
// chose. Un objet délibérément NON enraciné pendant une allocation doit être
// collecté sous le mode stress, et survivre sans lui.
//
// Ce test n'était pas décidable avant l'ajout de la génération de handle :
// l'emplacement libéré était immédiatement réattribué à l'objet suivant, si
// bien que le handle orphelin désignait le NOUVEAU locataire et paraissait
// vivant. La génération transforme ce cas en handle invalide.
func TestT3_1_StressFindsMissingRoot(t *testing.T) {
	alloc := func(stress bool) bool {
		h := NewHeap()
		h.SetStress(stress)
		orphan := h.NewObject() // délibérément non enraciné
		h.NewObject()           // déclenche une collecte en mode stress
		return h.Get(orphan) != nil
	}
	if !alloc(false) {
		t.Error("hors mode stress, un objet non enraciné disparaît dès la seconde allocation")
	}
	if alloc(true) {
		t.Error("en mode stress, un objet non enraciné SURVIT : le mode ne collecte pas, " +
			"et l'instrument T3.1 ne mesure rien")
	}
}

// ─── T3.3 : invariant de tas ────────────────────────────────────────────────

func TestT3_3_HeapInvariant(t *testing.T) {
	h := NewHeap()
	h.SetStress(true)

	var root Value = Undefined
	h.AddRoot(&root)
	buildGraph(h, &root, 3, 3)

	for i := 0; i < 20; i++ {
		h.Collect()
		if err := h.CheckHeap(); err != nil {
			t.Fatalf("collecte %d : %v", i, err)
		}
	}
}

// TestT3_3_InvariantDetectsDanglingHandle prouve que le vérificateur mesure.
// Un handle délibérément rendu pendant est signalé.
func TestT3_3_InvariantDetectsDanglingHandle(t *testing.T) {
	h := NewHeap()
	var root Value = Undefined
	h.AddRoot(&root)

	a := h.NewObject()
	root = ObjectValue(a)
	b := h.NewObject() // non enraciné
	h.SetProperty(a, k(h, "ref"), ObjectValue(b))

	if err := h.CheckHeap(); err != nil {
		t.Fatalf("invariant rompu avant toute falsification : %v", err)
	}

	// Falsification : libérer b sans nettoyer la référence portée par a.
	h.objs[b.Index()] = nil
	h.gens[b.Index()]++
	h.free = append(h.free, b.Index())

	if err := h.CheckHeap(); err == nil {
		t.Error("le vérificateur n'a pas vu un handle pendant : il ne mesure rien")
	} else {
		t.Logf("détecté : %v", err)
	}
}

// ─── T3.4 : absence de fuite ────────────────────────────────────────────────

// TestT3_4_NoLeak exécute N cycles d'allocation et de collecte sur un programme
// fermé. Le nombre d'objets vivants après le dernier cycle doit revenir à celui
// du premier — le seuil est nommé dans le test, jamais implicite.
func TestT3_4_NoLeak(t *testing.T) {
	const cycles = 200
	const tolerance = 0 // un programme fermé ne doit RIEN retenir de plus

	h := NewHeap()
	var root Value = Undefined
	h.AddRoot(&root)

	measure := func() int {
		root = Undefined
		buildGraph(h, &root, 3, 3)
		root = Undefined
		h.Collect()
		return h.Live()
	}

	first := measure()
	for i := 0; i < cycles; i++ {
		measure()
	}
	last := measure()

	if diff := last - first; diff > tolerance {
		t.Errorf("après %d cycles : %d objets vivants contre %d au premier cycle "+
			"(écart %d, tolérance %d)", cycles, last, first, diff, tolerance)
	}
	t.Logf("%d cycles : %d objets vivants au premier, %d au dernier ; %d collectes, %d balayés",
		cycles, first, last, h.Collections, h.Swept)
}

// TestT3_4_MemoryReturnsToRuntime vérifie que le tas ne croît pas indéfiniment
// en emplacements : les handles libérés sont réutilisés.
func TestT3_4_MemoryReturnsToRuntime(t *testing.T) {
	h := NewHeap()
	var root Value = Undefined
	h.AddRoot(&root)

	for i := 0; i < 50; i++ {
		root = Undefined
		buildGraph(h, &root, 3, 3)
	}
	root = Undefined
	h.Collect()

	slots := len(h.objs)
	for i := 0; i < 50; i++ {
		root = Undefined
		buildGraph(h, &root, 3, 3)
	}
	root = Undefined
	h.Collect()

	// Le nombre d'emplacements est un POINT HAUT : il dépend de l'instant où la
	// collecte automatique tombe par rapport à la construction du graphe, et
	// deux exécutions d'une même charge n'y tombent pas au même endroit. Exiger
	// zéro croissance mesurerait donc le hasard du seuil, pas la réutilisation
	// des handles. La borne est nommée.
	const growthTolerance = 5 // pour cent
	if grown := len(h.objs) - slots; grown*100 > slots*growthTolerance {
		t.Errorf("le tas est passé de %d à %d emplacements sur une charge identique "+
			"(croissance %d, tolérance %d %%) : les handles libérés ne sont pas réutilisés",
			slots, len(h.objs), grown, growthTolerance)
	}
	t.Logf("emplacements : %d après la première charge, %d après la seconde", slots, len(h.objs))
	runtime.GC()
}

// ─── T3.5 : chaînes internées et enracinement ───────────────────────────────

// TestT3_5_InternedStringsSurvive vérifie qu'une chaîne référencée depuis une
// forme reste utilisable après collecte. Les clés vivent dans les formes, pas
// dans les objets : une collecte d'objets ne doit pas les emporter.
func TestT3_5_InternedStringsSurvive(t *testing.T) {
	h := NewHeap()
	h.SetStress(true)

	var root Value = Undefined
	h.AddRoot(&root)

	obj := h.NewObject()
	root = ObjectValue(obj)
	names := []string{"alpha", "bêta", "😀clé", "�"}
	for i, n := range names {
		h.SetProperty(obj, k(h, n), Int(int32(i)))
	}

	for i := 0; i < 10; i++ {
		h.Collect()
	}

	for i, n := range names {
		v, ok := h.GetOwnProperty(obj, str.FromGo(n))
		if !ok || int(v.ToInt()) != i {
			t.Errorf("clé %q après collecte : %v, %v", n, v, ok)
		}
	}
	if err := h.CheckHeap(); err != nil {
		t.Errorf("invariant : %v", err)
	}
}

// ─── Valeurs ────────────────────────────────────────────────────────────────

func TestValueHandleRoundTrip(t *testing.T) {
	for _, hd := range []Handle{1, 2, 42, 1 << 20, (1 << 32) - 1} {
		v := ObjectValue(hd)
		if !v.IsObject() {
			t.Errorf("handle %d : la valeur n'est pas une référence d'objet", hd)
		}
		if got := v.Handle(); got != hd {
			t.Errorf("handle %d : aller-retour rend %d", hd, got)
		}
	}
	if got := ObjectValue(NoHandle); got != Null {
		t.Errorf("le handle nul devrait rendre Null, obtenu %v", got)
	}
	if got := Int(5).Handle(); got != NoHandle {
		t.Errorf("un entier ne porte pas de handle, obtenu %d", got)
	}
}

func TestObjKindEnumIsDense(t *testing.T) {
	for kk := ObjKind(0); int(kk) < NumObjKinds; kk++ {
		if kk.String() == "ObjKind(?)" {
			t.Errorf("ObjKind %d sans nom : l'énumération a un trou", kk)
		}
	}
}

// ─── Charge aléatoire ───────────────────────────────────────────────────────

// TestRandomizedGCAgreement exécute une charge aléatoire déterministe dans les
// deux modes et exige des états finaux identiques.
func TestRandomizedGCAgreement(t *testing.T) {
	run := func(stress bool) (int, string) {
		h := NewHeap()
		h.SetStress(stress)
		var root Value = Undefined
		h.AddRoot(&root)

		container := h.NewObject()
		root = ObjectValue(container)

		rnd := rand.New(rand.NewSource(20260829))
		for step := 0; step < 3000; step++ {
			switch rnd.Intn(4) {
			case 0:
				child := h.NewObject()
				h.SetProperty(container, k(h, fmt.Sprintf("p%d", rnd.Intn(16))), ObjectValue(child))
			case 1:
				arr := h.NewArray(rnd.Intn(8))
				h.SetProperty(container, k(h, fmt.Sprintf("a%d", rnd.Intn(8))), ObjectValue(arr))
			case 2:
				h.SetProperty(container, k(h, fmt.Sprintf("p%d", rnd.Intn(16))), Int(int32(step)))
			default:
				s := h.NewStringObject(str.FromGo(fmt.Sprintf("s%d", step)))
				h.SetProperty(container, k(h, fmt.Sprintf("t%d", rnd.Intn(4))), ObjectValue(s))
			}
		}
		h.Collect()

		var digest string
		for _, key := range h.MustGet(container).Shape().Keys() {
			v, _ := h.GetOwnProperty(container, key)
			if v.IsObject() {
				o := h.Get(v.Handle())
				if o == nil {
					digest += key.GoString() + ":LIBÉRÉ "
					continue
				}
				digest += key.GoString() + ":" + o.Kind().String() + " "
			} else {
				digest += key.GoString() + ":" + v.String() + " "
			}
		}
		return h.Live(), digest
	}

	liveA, digestA := run(false)
	liveB, digestB := run(true)

	if digestA != digestB {
		t.Errorf("empreinte finale différente selon le mode\n  normal : %s\n  stress : %s",
			digestA, digestB)
	}
	if liveA != liveB {
		t.Errorf("%d objets vivants en mode normal, %d en mode stress", liveA, liveB)
	}
	t.Logf("charge de 3000 pas : %d objets vivants dans les deux modes", liveA)
}
