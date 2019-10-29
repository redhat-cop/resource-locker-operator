package controller

import (
	"github.com/redhat-cop/resource-locker-operator/pkg/controller/resourcelocker"
)

func init() {
	// AddToManagerFuncs is a list of functions to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, resourcelocker.Add)
}
