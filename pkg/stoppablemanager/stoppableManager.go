package stoppablemanager

import (
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("stoppable_manager")

type StoppableManager struct {
	started bool
	manager.Manager
	stopChannel chan struct{}
}

func (sm *StoppableManager) Stop() {
	if !sm.started {
		return
	}
	close(sm.stopChannel)
	sm.started = false
}

func (sm *StoppableManager) Start() {

	if sm.started {
		return
	}
	go sm.Manager.Start(sm.stopChannel)
	sm.started = true
}

func NewStoppableManager(config *rest.Config, options manager.Options) (StoppableManager, error) {
	manager, err := manager.New(config, options)
	if err != nil {
		return StoppableManager{}, err
	}
	return StoppableManager{
		Manager:     manager,
		stopChannel: make(chan struct{}),
	}, nil
}
