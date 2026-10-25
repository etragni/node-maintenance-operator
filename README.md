# nodemaintenance-operator

Operatore Kubernetes didattico che rende dichiarativa la manutenzione di un nodo
worker: una risorsa `NodeMaintenance` mette il nodo in cordon ed evacua i Pod
(come `kubectl drain`); cancellare la risorsa rimette il nodo schedulabile.

Codice scritto per la tesi "Operator e Controller in Kubernetes".

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
