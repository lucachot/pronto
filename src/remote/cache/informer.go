package cache

import (
	"context"
	"log"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

type ApiInformer struct {
    onChange        func(count int)
    ctx             context.Context
    controller      cache.Controller
}

func (ai *ApiInformer) SetOnChange(onChange func(count int)) {
    ai.onChange = onChange
}

func (ai *ApiInformer) AddPod(obj interface{}) {
    pod := obj.(*corev1.Pod)
    if pod.Status.Phase == corev1.PodRunning {
        log.Printf("(api) %s running on this node", pod.Name)
        ai.onChange(1)
    }
}
func (ai *ApiInformer) UpdatePod(oldObj, newObj interface{}) {
    oldPod := oldObj.(*corev1.Pod)
    newPod := newObj.(*corev1.Pod)

    // If phase changes, update counter
    if oldPod.Status.Phase != corev1.PodRunning && newPod.Status.Phase == corev1.PodRunning {
        log.Printf("(api) %s running on this node", oldPod.Name)
        ai.onChange(1)
    } else if oldPod.Status.Phase == corev1.PodRunning && newPod.Status.Phase != corev1.PodRunning {
        log.Printf("(api) %s terminated", oldPod.Name)
        ai.onChange(-1)
    }
}

func (ai *ApiInformer) DeletePod(obj interface{}) {
    pod, ok := obj.(*corev1.Pod)
    if !ok {
        tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
        if !ok {
            return
        }
        pod, ok = tombstone.Obj.(*corev1.Pod)
        if !ok {
            return
        }
    }
    if pod.Status.Phase == corev1.PodRunning {
        log.Printf("(api) %s terminated", pod.Name)
        ai.onChange(-1)
    }
}

func NewApiInformer(ctx context.Context, client kubernetes.Interface, nodeName string) PodCountInformer {
    log.Print("created api informer")
    lw := cache.NewListWatchFromClient(
        client.CoreV1().RESTClient(),
        "pods",
        corev1.NamespaceAll,
        fields.OneTermEqualSelector("spec.nodeName", nodeName),
    )

    ai := &ApiInformer{
        ctx:    ctx,
        onChange: func(count int) {},
    }

    _, controller := cache.NewInformerWithOptions(cache.InformerOptions{
        ListerWatcher: lw,
        ObjectType: &corev1.Pod{},
        Handler: cache.ResourceEventHandlerFuncs{
            AddFunc: ai.AddPod,
            UpdateFunc: ai.UpdatePod,
            DeleteFunc: ai.DeletePod,
        },
    })

    ai.controller = controller

    return ai
}

func (ai *ApiInformer) Start() {
    go ai.controller.Run(ai.ctx.Done())
}
