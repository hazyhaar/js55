# DIRECTIVES — pkg/js55

**ID HPM55 :** `01a03f07-1c41-7e0c-97a6-1202ef5f51d1`  
**Parent HPM55 :** `019fd633-6735-705c-8c57-e09608dca298` (`c2simd`)  
**Intention :** Runtime et environnement d'exécution JavaScript souverain pur Go 1.27 (0-CGO, multi-tenant massif, intégration Netpoller/Goroutines Go, isolats légers avec metering CPU/RAM, APIs Node.js 20/22+ & Web Standards natives, pont zero-copy Buffer $\leftrightarrow$ `[]byte`, accélération hybride SIMD/JIT).

---

## 1. Principes Directeurs & Cadre Doctrinal

Le module `js55` constitue le socle d'exécution JavaScript souverain de l'écosystème Horos, conçu pour remplacer intégralement Node.js, Deno et les runtimes basés sur V8 au sein des applications et micro-services distribués.

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│                            APPLICATION HÔTE GO 1.27                              │
│  ┌────────────────────────────────────────────────────────────────────────────┐  │
│  │                              pkg/js55/pool                                 │  │
│  │          (Gestionnaire de réserve d'isolats chauds & recyclage)            │  │
│  └─────────────────────────────────────┬──────────────────────────────────────┘  │
│                                        │                                         │
│                                        ▼                                         │
│  ┌────────────────────────────────────────────────────────────────────────────┐  │
│  │                            pkg/js55/isolate                                │  │
│  │  ┌─────────────────────────┐ ┌────────────────────────┐ ┌───────────────┐ │  │
│  │  │ Gas Meter / Quota CPU   │ │ Allocateur Heap / Quota│ │ Timers Roue Go │ │  │
│  │  └─────────────────────────┘ └────────────────────────┘ └───────────────┘ │  │
│  │  ┌──────────────────────────────────────────────────────────────────────┐  │  │
│  │  │                           pkg/js55/engine                            │  │  │
│  │  │      (Interpréteur Bytecode Pur Go / Pile d'Exécution / Scope)       │  │  │
│  │  └──────────────────────────────────┬───────────────────────────────────┘  │  │
│  │                                     │                                      │  │
│  │                                     ▼                                      │  │
│  │  ┌──────────────────────────────────────────────────────────────────────┐  │  │
│  │  │                           pkg/js55/runtime                           │  │  │
│  │  │  ┌───────────────┐ ┌───────────────┐ ┌──────────────┐ ┌────────────┐ │  │  │
│  │  │  │  fs (Sandbox) │ │ http / fetch  │ │ crypto/subtle│ │   buffer   │ │  │  │
│  │  │  └───────────────┘ └───────────────┘ └──────────────┘ └────────────┘ │  │  │
│  │  │  ┌───────────────┐ ┌───────────────┐ ┌──────────────┐ ┌────────────┐ │  │  │
│  │  │  │ stream/events │ │ timers/wheel  │ │  path / os   │ │  console   │ │  │  │
│  │  │  └───────────────┘ └───────────────┘ └──────────────┘ └────────────┘ │  │  │
│  │  └──────────────────────────────────┬───────────────────────────────────┘  │  │
│  └─────────────────────────────────────┼──────────────────────────────────────┘  │
│                                        │                                         │
│                                        ▼                                         │
│  ┌────────────────────────────────────────────────────────────────────────────┐  │
│  │              INFRASTRUCTURE SYSTÈME & ACCÉLÉRATION MATÉRIELLE              │  │
│  │  ┌─────────────────────┐ ┌────────────────────┐ ┌───────────────────────┐  │  │
│  │  │ Go 1.27 Netpoller   │ │ c2pkg/c2jit (JIT)  │ │ c2pkg/c2archsimd      │  │  │
│  │  │ (E/S non-bloquantes)│ │ (Boucles chaudes)  │ │ (Vectorisation AVX2)  │  │  │
│  │  └─────────────────────┘ └────────────────────┘ └───────────────────────┘  │  │
│  └────────────────────────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────────────────────────┘
```

1. **Pur Go 1.27 & Zéro Dépendance CGO (`CGO_ENABLED=0`) :**  
   Aucune inclusion de code C, aucune liaison dynamique (`libv8.so`, `libnode.so`, `libuv.so`), aucun binaire externe. Compilation statique universelle, compatibilité totale avec `GOEXPERIMENT=simd`.
2. **Concurrence Native M:N & Remplacement de `libuv` :**  
   Substitution de la boucle d'événements mono-threadée classique par le planificateur de Goroutines et le *Netpoller* non-bloquant de Go. Les opérations d'E/S (réseau, disque) sont réparties sur l'ensemble des cœurs processeur sans bloquer les isolats.
3. **Multi-Tenancy Ultra-Léger :**  
   Capacité d'héberger plus de 10 000 isolats JavaScript simultanés et indépendants dans un seul processus Go. Empreinte mémoire au repos inférieure à $500\text{ Ko}$ par contexte.
4. **Cloisonnement & Sandboxing Déterministe :**  
   Chaque isolat est strictement étanche (zéro état global partagé). Contrôle absolu des ressources via un compteur d'instructions atomique (*gas meter*), des plafonds de mémoire vive (*heap quota*), une arborescence de fichiers restreinte (*VirtualFS*) et une liste blanche d'adresses réseau (*network allowlist*).
5. **Pont Zero-Copy Buffer $\leftrightarrow$ `[]byte` :**  
   Partage direct de mémoire entre les tranches d'octets Go et les objets `Buffer` / `Uint8Array` JavaScript sans duplication mémoire lors des lectures/écritures réseau ou disque.
6. **Accélération Hybride ARCHTIME & Hot-Spots :**  
   - Exécution instantanée par interpréteur de bytecode pur Go.
   - Compilateur AOT statique [`cmd/c2jsc`](file:///devhoros/c2simd/cmd/c2jsc/) pour les modules immutables résolus avant compilation.
   - Détection des boucles intensives et bascule dynamique vers le micro-assembleur JIT [`c2pkg/c2jit`](file:///devhoros/c2simd/c2pkg/c2jit/) pour l'émission de code machine natif AVX2/x86-64/ARM64.
   - Primitives vectorielles déléguées aux bibliothèques [`c2pkg`](file:///devhoros/c2simd/c2pkg/) (`c2archsimd`, `c2base64`, `c2chacha8`, `c2poly1305`).

---

## 2. Découpage Modulaire Exhaustif

Le package `js55` est structuré en cinq sous-systèmes modulaires autonomes :

```
/devhoros/pkg/js55/
├── DIRECTIVES.md       # Document de référence architectural et contractuel
├── doc.go              # Documentation du package racine
├── js55.go             # Façade publique unifiée et constructeurs principaux
├── js55_test.go        # Tests d'intégration et scénarios de validation
├── engine/             # Moteur d'exécution, AST, machine virtuelle et bytecode
│   ├── ast.go
│   ├── bytecode.go
│   ├── compiler.go
│   ├── context.go
│   ├── gc.go
│   ├── opcode.go
│   ├── runtime_state.go
│   ├── value.go
│   └── vm.go
├── isolate/            # Gestionnaire de multi-tenancy, quotas et bac à sable
│   ├── config.go
│   ├── gas_meter.go
│   ├── heap_tracker.go
│   ├── interrupt.go
│   ├── isolate.go
│   ├── limits.go
│   └── sandbox.go
├── runtime/            # Émulation complète des APIs Node.js 20/22+ et Web Standards
│   ├── buffer/         # Module Buffer & Uint8Array zero-copy
│   │   ├── buffer.go
│   │   └── zero_copy.go
│   ├── crypto/         # Web Crypto API & Node crypto via c2pkg
│   │   ├── cipher.go
│   │   ├── crypto.go
│   │   ├── hash.go
│   │   └── random.go
│   ├── events/         # EventEmitter & EventTarget
│   │   ├── abort_controller.go
│   │   └── event_emitter.go
│   ├── fs/             # Filesystem synchrone / asynchrone avec sandbox
│   │   ├── fs.go
│   │   ├── promises.go
│   │   └── vfs.go
│   ├── http/           # Client / Serveur HTTP & Fetch WHATWG via Go Netpoller
│   │   ├── client.go
│   │   ├── fetch.go
│   │   ├── headers.go
│   │   ├── request.go
│   │   ├── response.go
│   │   └── server.go
│   ├── stream/         # Streams3 & WHATWG Streams (Readable, Writable, Transform)
│   │   ├── pipe.go
│   │   ├── readable.go
│   │   ├── transform.go
│   │   └── writable.go
│   └── timers/         # Timers non-dérivants sur roue hiérarchique Go
│       ├── immediate.go
│       ├── interval.go
│       ├── timeout.go
│       └── wheel.go
├── bridge/             # Passerelle bidirectionnelle Go <-> JS
│   ├── convert.go
│   ├── error.go
│   ├── function.go
│   ├── reflect.go
│   └── struct.go
└── pool/               # Gestionnaire de réserve d'isolats chauds (Warm Pool)
    ├── metrics.go
    ├── pool.go
    ├── recycle.go
    └── stats.go
```

### 2.1. Sous-système `engine/` (Noyau d'Interprétation)
- **Machine Virtuelle de Bytecode :** Interpréteur à pile 64 bits sans allocation sur le chemin critique. Boucle d'opcodes vectorisée avec registre d'instructions déroulé.
- **Gestion des Types `Value` :** Représentation compacte par étiquetage de pointeurs (*NaN-tagging* ou tagged pointer 64-bit `uint64`), permettant de stocker les entiers 32 bits, flottants double précision, booléens, pointeurs d'objets, symboles et types sentinelles (`undefined`, `null`) dans un seul mot machine sans allocation sur le tas Go.
- **Ramasse-Miettes Intégré & Coopératif :** Suivi des références cycliques JS sans bloquer le ramasse-miettes de Go. Libération déterministe de la mémoire des isolats à leur fermeture.
- **Support ECMAScript :** Conformité ES2023+ (fonctions fléchées, fermetures lexicales, générateurs, `async`/`await`, `Promise`, `BigInt`, `Map`, `Set`, `WeakMap`, `WeakSet`, `Proxy`, `Reflect`).

### 2.2. Sous-système `isolate/` (Supervision Multi-Tenant & Quotas)
- **Compteur d'Instructions CPU (*Gas Metering*) :** Décrémentation d'un compteur atomique à chaque saut de boucle et appel de fonction. Déclenchement d'une interruption déterministe `ErrExecutionQuotaExceeded` sans arrêt du processus Go.
- **Suivi de la Mémoire Vive (*Heap Tracking*) :** Encadrement strict de la mémoire allouée par le tas JS via `heap_tracker.go`. Émission d'une erreur `ErrMemoryLimitExceeded` dès que le seuil configuré est franchi.
- **Bac à Sable (*Sandbox*) :**
  - Fichiers : Accès restreint à un sous-répertoire (`VirtualFS` ou chroot applicatif) ; options de montage en lecture seule.
  - Réseau : Filtrage des adresses IP / domaines autorisés (`NetworkAllowlist`).
  - Environnement : Isolation des variables d'environnement (`process.env`).
- **Interruption et Mise en Pause :** Primitives `Interrupt()`, `Pause()`, `Resume()`, permettant de suspendre ou terminer un isolat depuis n'importe quelle goroutine.

### 2.3. Sous-système `runtime/` (Émulation Node.js 20/22+ & Web Standards)
- **`buffer` / `Uint8Array` :**
  - Pont zero-copy : Un `Buffer` JavaScript encapsule directement un pointeur vers un `[]byte` Go sous-jacent.
  - Encodages supportés : `utf8`, `ascii`, `utf16le`, `base64`, `hex`, `binary`.
  - Opérations accélérées : Décodage Base64 et Hex via [`c2pkg/c2base64`](file:///devhoros/c2simd/c2pkg/c2base64/) et `c2archsimd`.
- **`fs` & `fs/promises` :**
  - Émulation intégrale : `readFile`, `writeFile`, `appendFile`, `stat`, `lstat`, `readdir`, `mkdir`, `rm`, `unlink`, `rename`, `copyFile`, `watch`.
  - Intégration non-bloquante : Les versions asynchrones renvoient une `Promise` résolue sur une Goroutine d'arrière-plan du pool Go.
- **`http`, `https` & `fetch` (Standard WHATWG) :**
  - Moteur HTTP adossé à `net/http` et au *Netpoller* Go.
  - Support de `fetch(url, init)` avec streaming du corps (`ReadableStream`), résolution DNS non-bloquante et gestion de `AbortController`.
  - Serveur HTTP : `http.createServer((req, res) => ...)` dispatchant chaque requête sur les Goroutines Go avec un débit élevé.
- **`crypto` & `crypto/subtle` (Web Crypto) :**
  - Algorithmes symétriques : ChaCha20-Poly1305, AES-128-GCM, AES-256-GCM via [`c2pkg/c2chacha8`](file:///devhoros/c2simd/c2pkg/c2chacha8/), [`c2pkg/c2poly1305`](file:///devhoros/c2simd/c2pkg/c2poly1305/) et primitives Go standard.
  - Fonctions de hachage : SHA-256, SHA-512, SHA-3, BLAKE3.
  - Nombres aléatoires : `crypto.randomBytes(size)`, `crypto.getRandomValues(typedArray)` connectés à `crypto/rand`.
  - Signatures & Dérivations : HMAC, PBKDF2, Ed25519, ECDSA.
- **`stream` :**
  - Backpressure matérielle : Les flux `Readable` et `Writable` utilisent des canaux Go typés (`chan []byte`) ou `io.Pipe`, empêchant tout débordement mémoire dans le tas JS.
- **`timers` :**
  - Roue de temporisation hiérarchique (*Hierarchical Timing Wheel*) en Go : Évite les dérives temporelles lors de la mise en pause/reprise de l'isolat.
  - Primitives supportées : `setTimeout`, `clearTimeout`, `setInterval`, `clearInterval`, `setImmediate`, `clearImmediate`, `queueMicrotask`.
- **`events` :**
  - Implémentation standard de `EventEmitter` et `EventTarget`.
  - Gestion des signaux d'annulation `AbortController` et `AbortSignal`.

### 2.4. Sous-système `bridge/` (Passerelle Go $\leftrightarrow$ JS)
- **Conversion Réflexive Déterministe :** Conversion sans friction des structures, tranches, tables de hachage (`map[string]any`) Go en objets, tableaux et dictionnaires JS.
- **Appels de Fonctions :** Possibilité d'exposer n'importe quelle fonction Go `func(ctx context.Context, args ...any) (any, error)` comme fonction synchrone ou asynchrone (retournant une `Promise`) en JavaScript.
- **Propagation d'Erreurs :** Traduction bidirectionnelle des paniques / erreurs Go en exceptions JavaScript (`Error`, `TypeError`, `RangeError`) et réciproquement.

### 2.5. Sous-système `pool/` (Gestionnaire de Réserve d'Isolats Chauds)
- **Recyclage Rapide (*Warm Pool*) :** Réinitialisation d'un isolat en moins de $50\ \mu\text{s}$ via `Reset()`, évitant le coût de réallocation du tas et de réévaluation du code de base.
- **Dimensionnement Dynamique :** Capacité minimale et maximale d'isolats actifs, éviction LRU des isolats inactifs, surveillance de la santé globale.

---

## 3. Accords de Niveau de Service (SLAs) & Performances

| Métrique | Seuil Cible | Seuil Critique (Gate CI) | Méthode de Mesure |
| :--- | :--- | :--- | :--- |
| **Démarrage à Froid (*Cold Boot*)** | $< 300\ \mu\text{s}$ | $\le 1{,}0\text{ ms}$ | `BenchmarkIsolateColdBoot` |
| **Recyclage d'Isolat (*Warm Reset*)**| $< 30\ \mu\text{s}$ | $\le 50\ \mu\text{s}$ | `BenchmarkIsolateWarmReset` |
| **Empreinte RAM de Base (par Isolat)** | $< 350\text{ Ko}$ | $\le 500\text{ Ko}$ | `TestIsolateMemoryFootprint` |
| **Passage à l'Échelle Multi-Tenant** | $10\,000$ isolats / $4\text{ Go}$ RAM | $10\,000$ isolats / $5\text{ Go}$ RAM | `TestMultiTenant10kIsolates` |
| **Fuite Mémoire (10 000 cycles création/destruction)** | $0\text{ octet}$ net | $0\text{ fuite}$ détectée (`pprof`) | `TestZeroMemoryLeakLifecycle` |
| **Débit I/O JSON / HTTP (par cœur)** | $> 120\,000\text{ req/s}$ | $\ge 80\,000\text{ req/s}$ | `BenchmarkHTTPServerThroughput` |
| **Traversée Pont Zero-Copy Buffer (1 Mo)** | $< 100\text{ ns}$ ($0\text{ B/op}$) | $\le 200\text{ ns}$ ($0\text{ B/op}$) | `BenchmarkZeroCopyBufferTransfer` |
| **Précision du Compteur CPU (Gas Meter)** | Dérive $< 0{,}1\,\%$ | Dérive $\le 0{,}5\,\%$ | `TestGasMeterDeterministicPrecision` |

---

## 4. Spécification de l'API Publique Go

Le contrat d'embarquement public de `js55` dans toute application Go respecte l'interface suivante :

```go
package js55

import (
	"context"
	"io"
	"time"
)

// Config définit les paramètres de configuration et limites d'un Isolate.
type Config struct {
	// Limites de ressources
	MaxMemoryBytes   uint64        // Quota mémoire du tas JS (ex: 8 * 1024 * 1024)
	MaxCPUGas        uint64        // Quota d'instructions de bytecode (0 = illimité)
	ExecutionTimeout time.Duration // Temps d'exécution maximal (0 = illimité)

	// Sécurité et bac à sable
	AllowedFSPaths   []string      // Chemins du système de fichiers autorisés (lecture/écriture)
	ReadOnlyFSPaths  []string      // Chemins autorisés en lecture seule
	NetworkAllowlist []string      // Domaines/hôtes autorisés pour fetch / http
	DisableNativeNet bool          // Désactive complètement l'accès réseau

	// Environnement
	Environment map[string]string  // Variables transmises à process.env
	Stdout      io.Writer          // Redirection de console.log (default: os.Stdout)
	Stderr      io.Writer          // Redirection de console.error (default: os.Stderr)

	// Optimisation
	EnableJIT   bool               // Activation de l'accélération c2pkg/c2jit
	PreloadCode []Script           // Scripts précompilés injectés au démarrage
}

// Script représente un code source JavaScript précompilé en bytecode.
type Script interface {
	ID() string
	Bytecode() []byte
}

// Engine est l'instance globale du moteur, gérant les tables de types et compilateurs.
type Engine struct {
	// Champs internes non exportés
}

// NewEngine initialise un nouveau moteur js55.
func NewEngine() (*Engine, error)

// Compile précompile un script JS en bytecode réutilisable.
func (e *Engine) Compile(filename, source string) (Script, error)

// Isolate représente une instance isolée d'exécution JavaScript.
type Isolate struct {
	// Champs internes non exportés
}

// NewIsolate instancie un nouvel isolat avec une configuration dédiée.
func (e *Engine) NewIsolate(cfg Config) (*Isolate, error)

// Méthodes d'exécution sur l'Isolate
func (iso *Isolate) Eval(ctx context.Context, code string) (Value, error)
func (iso *Isolate) RunScript(ctx context.Context, script Script) (Value, error)
func (iso *Isolate) Call(ctx context.Context, funcName string, args ...any) (Value, error)

// Gestion des valeurs et du pont Go <-> JS
func (iso *Isolate) Global() Object
func (iso *Isolate) SetGlobal(name string, val any) error
func (iso *Isolate) ExposeFunction(name string, fn any) error

// Primitives de contrôle et cycle de vie
func (iso *Isolate) Interrupt()
func (iso *Isolate) Pause() error
func (iso *Isolate) Resume() error
func (iso *Isolate) Reset() error
func (iso *Isolate) Close() error

// Métriques et consommation
func (iso *Isolate) ConsumedGas() uint64
func (iso *Isolate) AllocatedMemory() uint64

// Value représente une valeur JavaScript (étiquetée NaN 64-bit).
type Value interface {
	IsUndefined() bool
	IsNull() bool
	IsBool() bool
	IsNumber() bool
	IsString() bool
	IsObject() bool
	IsArray() bool
	IsFunction() bool
	IsPromise() bool
	IsBuffer() bool

	Bool() bool
	Int64() int64
	Float64() float64
	String() string
	Bytes() []byte
	Export() any
}

// Object représente un objet JavaScript manipulable depuis Go.
type Object interface {
	Value
	Get(key string) (Value, error)
	Set(key string, val any) error
	Delete(key string) error
	Keys() []string
}

// Pool gère une réserve d'isolats chauds pour charges transactionnelles.
type Pool struct {
	// Champs internes non exportés
}

// NewPool crée une réserve d'isolats dimensionnée.
func NewPool(e *Engine, cfg Config, initialSize, maxSize int) (*Pool, error)
func (p *Pool) Acquire(ctx context.Context) (*Isolate, error)
func (p *Pool) Release(iso *Isolate)
func (p *Pool) Stats() PoolStats
func (p *Pool) Close() error
```

---

## 5. Matrice de Tests & Scénarios de Validation

Toute implémentation ou modification du package `js55` doit satisfaire sans exception la matrice de tests d'acceptation ci-dessous :

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│                         MATRICE DE VALIDATION - js55                             │
├───────────────────┬──────────────────────────────────┬───────────────────────────┤
│ Domaine de Test   │ Fichier de Test Cible            │ Critère de Réussite       │
├───────────────────┼──────────────────────────────────┼───────────────────────────┤
│ Conformité ES2023 │ engine/ecma_test.go              │ 100% assertions validées  │
│ Zero-Copy Buffer  │ runtime/buffer/buffer_test.go    │ 0 alloc sur conversions   │
│ Filesystem Sandbox│ runtime/fs/fs_test.go            │ Blocage évasion répertoire│
│ Netpoller HTTP    │ runtime/http/http_test.go        │ 10k req non-bloquantes    │
│ Cryptographie C2  │ runtime/crypto/crypto_test.go    │ Parité bit-exacte c2pkg   │
│ Timers Non-Dérive │ runtime/timers/timers_test.go    │ Écart temporel < 1 ms     │
│ Gas Metering CPU  │ isolate/gas_meter_test.go        │ Arrêt boucle infinie net  │
│ Quota Mémoire RAM │ isolate/heap_test.go             │ ErrMemoryLimit déclenchée │
│ Multi-Tenant 10k  │ isolate/multitenant_test.go      │ 10 000 isolats simultanés │
│ Zéro Fuite RAM    │ isolate/leak_test.go             │ 0 octet résiduel à blanc  │
│ Warm Pool         │ pool/pool_test.go                │ Reset < 50 µs sous charge │
│ Intégrité Races   │ go test -race ./...              │ Zéro conflit de données   │
└───────────────────┴──────────────────────────────────┴───────────────────────────┘
```

### Scénarios de Test Explicites

1. **Scénario `TestGasMeterInfiniteLoop` :**
   Exécution d'un script adverse contenant `while(true) {}`. L'isolat doit interrompre le traitement avec l'erreur `ErrExecutionQuotaExceeded` en moins de $5\text{ ms}$ et libérer la Goroutine hôte sans saturer le processeur.
2. **Scénario `TestMemoryQuotaBreach` :**
   Exécution d'un script allouant récursivement des tableaux géants (`let a = []; while(true) a.push(new Uint8Array(1024*1024));`). L'isolat doit être stoppé net dès le franchissement de `MaxMemoryBytes` sans impacter les autres isolats du processus.
3. **Scénario `TestZeroCopyBufferRoundtrip` :**
   Transmission d'un tranche d'octets Go `[]byte` de 16 Mo via `Buffer.from(slice)`. Modification directe d'un octet en JavaScript (`buf[0] = 0xAA`). Vérification immédiate que `slice[0] == 0xAA` en mémoire Go sans aucune allocation mémoire intermédiaire (`testing.AllocsPerRun == 0`).
4. **Scénario `TestAsyncHttpNetpoller` :**
   Exécution simultanée de 1 000 requêtes `fetch()` asynchrones vers un serveur HTTP Go local. Les promesses JavaScript doivent être résolues sans bloquer la boucle d'événements, avec une consommation mémoire linéaire et sans dépassement du nombre de threads système.
5. **Scénario `TestMultiTenant10kConcurrency` :**
   Instanciation de 10 000 isolats exécutant chacun une suite de calculs JSON et d'opérations d'horodatage. Contrôle sous `runtime.ReadMemStats` : l'occupation mémoire totale ne doit pas excéder $5\text{ Go}$ et aucun blocage de l'ordonnanceur Go ne doit survenir.

---

## 6. Interdictions Formelles & Règles Dures

1. **Interdiction de CGO :** Aucun fichier `.c`, `.h`, `.cpp` ou directive `import "C"` dans le sous-arbre `pkg/js55/`.
2. **Interdiction de `git add .` :** Tout ajout de fichier doit être explicite et vérifié par `horos55-commit-guard`.
3. **Interdiction de Variables Globales Mutables :** Aucun état global mutable partagé au niveau du package. Tout l'état réside dans les structures `Engine` ou `Isolate`.
4. **Interdiction de Panique Non Rattrapée :** Aucune panique Go issue du moteur JS ne doit se propager hors de l'isolat ; toutes les erreurs d'exécution doivent être converties en `error` Go ou `Value` d'exception JS.

---

## 7. Dogfooding & Chaîne de Réduction C++ vers Go 1.27 SIMD (LLVM IR)

### 7.1. Rôle Fondateur des Nœuds V8 dans HPM55 (`lots_v8` & `cartographie_v8_headers`)
Les 246 micro-lots de transpilation (`lots_v8`) et les 4 640 nœuds d'en-tête C++ V8 (`cartographie_v8_headers`) hébergés sous `js55` constituent le corpus de référence et le banc d'épreuve d'ingénierie le plus poussé pour la stack compilateur C2SIMD / Sgoiter.

### 7.2. Doctrine de Réduction par Assembleur Structuré (LLVM IR)
L'abaissement (*lowering*) du code source C++ de V8 ne s'effectue pas par une tentative fragile de rétro-traduction vers du C99 manuscrit ou pré-processé. La chaîne cible formellement l'émission de **LLVM IR** (*Low-Level Virtual Machine Intermediate Representation*) via Clang (`clang++ -S -emit-llvm`) :
1. **Aplatissement Sémantique :** LLVM IR élimine intégralement la complexité objet du C++ (métaprogrammation par templates, hiérarchies de classes, dispatch virtuel, constructeurs/destructeurs RAII implicites) en la traduisant en structures mémoire explicites, arithmétique de pointeurs et graphes de contrôle de flot en forme SSA (*Static Single Assignment*).
2. **Assembleur Structuré Typé :** En tant qu'assembleur structuré intermédiaire fortement typé à trois adresses, LLVM IR offre une représentation canonique sans ambiguïté.
3. **Pipeline Mécanique C2SIMD / Sgoiter :**
   $$\text{C++ V8 Upstream} \xrightarrow{\text{clang++}} \text{LLVM IR (SSA)} \xrightarrow{\text{sgoiter}} \text{Go 1.27 Register ABI (SIMD, 0-CGO)}$$
4. **Parité Bit-Exacte & Zéro Échappement :** Chaque composant transpilé est soumis au protocole canonique de dogfooding en 6 étapes ([`spec/PROTOCOLE_DOGFOODING.md`](file:///devhoros/c2simd/sgoiter/spec/PROTOCOLE_DOGFOODING.md)), validé contre oracle binaire compilé avec `gcc -O2` sous `GOEXPERIMENT=simd go test -race`.

