# 🚀 Kubernetes Operator Masterclass

Welcome to the definitive, production-grade guide to building Kubernetes Operators in Go.

This repository is not a collection of fragmented snippets. It is a comprehensive, heavily-architected masterclass designed to take you from a theoretical understanding of the Kubernetes Control Plane to deploying enterprise-grade, highly reliable Operators using the exact same patterns employed by Google, Red Hat, and CoreOS.

If you have ever struggled with the undocumented nuances of `client-go`, the asynchronous chaos of Informer caches, or the terrifying lifecycle of garbage collection, this is the book you have been waiting for.

---

## 📖 The Textbook

The theoretical deep-dives are located in the `book/` directory. These are not 2-sentence tutorials. Every chapter contains massive, pedagogical explanations, strict Go code implementations, and intricate ASCII diagrams detailing the exact flow of data through the Kubernetes API Server.

- [Chapter 1: Deconstructing the Control Plane](book/chapter01_k8s_architecture.md)
- [Chapter 2: The Gateway to the API: Mastering client-go](book/chapter02_client_go_basics.md)
- [Chapter 3: The Secret Sauce: Watches, Informers, Caches, and Workqueues](book/chapter03_informers_caches.md)
- [Chapter 4: Defining the Domain: CRDs, OpenAPI Validation, and DeepCopy](book/chapter04_crd_design.md)
- [Chapter 5: Taming the Chaos: The Controller-Runtime Framework](book/chapter05_controller_runtime.md)
- [Chapter 6: The Flat Reconciler: Guard Clauses and End-to-End Implementation](book/chapter06_building_operator.md)
- [Chapter 7: Garbage Collection: OwnerReferences and Custom Finalizers](book/chapter07_finalizers_gc.md)
- [Chapter 8: The Language of the Cluster: Status Subresources and Conditions](book/chapter08_status_conditions.md)
- [Chapter 9: The Pre-Flight Check: Mutating and Validating Admission Webhooks](book/chapter09_webhooks.md)
- [Chapter 10: The Crucible: Integration Testing with EnvTest](book/chapter10_testing_envtest.md)

---

## 💻 The Modular Codebase

Theory is useless without execution.

Inside the `hands_on/` directory, you will find a massive collection of Go projects. These are not monolithic scripts. Every project from Chapter 4 onwards strictly follows the industry-standard **Kubebuilder Directory Layout**:

```text
hands_on/ch08_status/
├── api/
│   └── v1/
│       └── database_types.go      <-- (OpenAPI Markers & DeepCopy logic)
├── cmd/
│   └── main.go                    <-- (Manager Bootstrap & Scheme Wiring)
├── internal/
│   └── controller/
│       └── status_controller.go   <-- (Pure Reconciler Logic & Guard Clauses)
├── go.mod
└── go.sum
```

Every single `main.go` file is fully runnable. Every single `Reconciler` is meticulously documented line-by-line, explaining *why* the code is written the way it is.

To run any example:
```bash
cd hands_on/ch08_status
go run cmd/main.go
```

## 🏗️ Architectural Philosophy

This repository strictly enforces the following design patterns:
1. **Level-Triggered Reconciliation:** We never assume the object passed to the queue is fresh. We always `Get()` the latest state from the Local Cache.
2. **The Flat Reconciler:** You will not find nested `if/else` statements in this repository. All business logic is flattened using early-return Guard Clauses.
3. **Strict DeepCopying:** We never mutate pointers retrieved from the Informer cache to prevent catastrophic global state corruption.

Dive in, and master the Control Plane.
