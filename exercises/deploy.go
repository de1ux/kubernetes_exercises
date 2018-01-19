package main

import (
	"fmt"
	"path/filepath"
	"time"

	apiv1 "k8s.io/api/extensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	v1 "k8s.io/client-go/kubernetes/typed/extensions/v1beta1"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"k8s.io/client-go/util/retry"
)

const (
	deployRunningThreshold     = time.Second * 10
	deployRunningCheckInterval = time.Second * 2
)

func main() {
	config, err := clientcmd.BuildConfigFromFlags("", filepath.Join(homedir.HomeDir(), ".kube", "config"))
	if err != nil {
		panic(err)
	}

	clientSet, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err)
	}

	dClient := clientSet.ExtensionsV1beta1().Deployments("default")

	// Get a copy of the current deployment
	originalDeployment, err := dClient.Get("redis", metav1.GetOptions{})
	if err != nil {
		panic(err)
	}

	// Verify the current containers in the pod are running
	if allRunning, err := podContainersRunning(clientSet, "redis"); !(allRunning && err == nil) {
		panic(fmt.Sprintf("Not all containers are currently running, or err: %s", err))
	}

	if err := deploy(dClient, "redis", func(deployment *apiv1.Deployment) {
		deployment.Spec.Template.Spec.Containers[0].Image = "redis:doesntexist"
	}); err != nil {
		panic(err)
	}

	err = waitForPodContainersRunning(clientSet, "redis")

	if err == nil {
		println("Deploy successful")
	}

	// Try rolling back
	if err := deploy(dClient, "redis", func(deployment *apiv1.Deployment) {
		deployment.Spec.Template.Spec.Containers[0].Image = originalDeployment.Spec.Template.Spec.Containers[0].Image
	}); err != nil {
		panic(err)
	}

	err = waitForPodContainersRunning(clientSet, "redis")
	if err != nil {
		panic(err)
	}
	println("Rolled back successfully!")
}

func deploy(dClient v1.DeploymentInterface, app string, op func(deployment *apiv1.Deployment)) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		result, err := dClient.Get(app, metav1.GetOptions{})
		if err != nil {
			panic(fmt.Errorf("Failed to get latest version of %s: %s", app, err))
		}

		op(result)

		_, updateErr := dClient.Update(result)
		return updateErr
	})
}

func waitForPodContainersRunning(clientSet *kubernetes.Clientset, app string) error {
	end := time.Now().Add(deployRunningThreshold)

	for true {
		<-time.NewTimer(deployRunningCheckInterval).C

		var err error
		running, err := podContainersRunning(clientSet, app)
		if running {
			return nil
		}

		if err != nil {
			println(fmt.Sprintf("Encountered an error checking for running pods: %s", err))
		}

		if time.Now().After(end) {
			return fmt.Errorf("Failed to get all running containers")
		}
	}
	return nil
}

func podContainersRunning(clientSet *kubernetes.Clientset, app string) (bool, error) {
	pods, err := clientSet.CoreV1().Pods("default").List(metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app=%s", app),
	})
	if err != nil {
		return false, err
	}

	for _, item := range pods.Items {
		for _, status := range item.Status.ContainerStatuses {
			if !status.Ready {
				return false, nil
			}
		}
	}
	return true, nil
}
