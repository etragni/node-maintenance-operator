# nodemaintenance-operator

Operatore Kubernetes didattico che rende dichiarativa la manutenzione di un nodo
worker: una risorsa `NodeMaintenance` mette il nodo in cordon ed evacua i Pod
(come `kubectl drain`); cancellare la risorsa rimette il nodo schedulabile.

Codice scritto per la tesi "Operator e Controller in Kubernetes".

## Stato della verifica (leggere prima di usarlo)

| Parte | Stato |
|---|---|
| `internal/maintenance` (logica di decisione pura, solo libreria standard) | Compilata, `go vet` pulito, test passati (10 casi in tabella + enumerazione esaustiva di 128 combinazioni di input). |
| `api/`, `internal/controller/`, `cmd/` | **Non compilati.** Sintassi controllata con `gofmt -e` e import controllati meccanicamente. Non c'è stato accesso ai moduli Go, quindi nessun type-check. Aspettarsi al massimo piccole correzioni al primo `go build`. |
| Firme dell'API di drain | Verificate sulla documentazione di `k8s.io/kubectl/pkg/drain` (versione v0.37.1): `RunCordonOrUncordon(drainer, node, desired)`, `RunNodeDrain(drainer, nodeName)`, campi di `drain.Helper`. |
| Comportamento su cluster reale | Eseguito dall'autore su `kind` (1 control-plane + 3 worker, Kubernetes v1.35.0): cordon, drain, uncordon alla cancellazione e finalizer funzionanti. Esito riferito dall'autore, non riprodotto da chi ha scritto il codice. Non ancora provati: rifiuto del control-plane e blocco da PodDisruptionBudget. |

Assunzioni: `controller-runtime` >= v0.15 (firme di `Watches` e `EnqueueRequestsFromMapFunc`).
Se la versione risolta da `go mod tidy` ne è diversa, adattare quelle due chiamate.

## Prerequisiti

Go >= 1.22, Docker, `kind`, `kubectl`, `controller-gen`
(`go install sigs.k8s.io/controller-tools/cmd/controller-gen@latest`).

## Esecuzione

```bash
make tidy        # risolve le dipendenze (genera go.sum)
make test        # test della logica di decisione
make kind        # cluster `kind` con 1 control-plane + 3 worker (nodi kind-control-plane, kind-worker, ...)
make generate    # DeepCopy
make install     # genera e applica la CRD
kubectl apply -f config/samples/demo-workload.yaml
make run         # avvia l'operatore in locale (usa il kubeconfig corrente)
```

In un secondo terminale:

```bash
kubectl get pods -o wide                       # stato iniziale
kubectl apply -f config/samples/nodemaintenance-worker2.yaml
kubectl get nodemaintenance -w                 # Cordoned -> Draining -> Drained
kubectl get nodes                              # kind-worker2: SchedulingDisabled
kubectl get pods -o wide                       # nessun Pod su kind-worker2
kubectl delete -f config/samples/nodemaintenance-worker2.yaml
kubectl get nodes                              # nodo di nuovo schedulabile
kubectl apply -f config/samples/nodemaintenance-control-plane.yaml
kubectl get nodemaintenance control-plane-test -o yaml   # phase: Failed
```

Gli output reali di questi comandi sono quelli da riportare nella tesi.

## Limiti noti (da dichiarare)

- Solo nodi worker: i nodi control-plane sono rifiutati.
- `DeleteEmptyDirData=false`: Pod con `emptyDir` bloccano il drain finché non intervene un umano (scelta di sicurezza).
- Alla cancellazione viene fatto l'uncordon solo se l'operatore aveva agito; un nodo già in cordon prima della richiesta viene comunque rimesso schedulabile.
- Il Pod con PodDisruptionBudget che non consente evizioni fa scadere il timeout di 30 s e il retry avviene con backoff.
