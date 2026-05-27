package central

import (
	"context"
	"fmt"
	"os"
	"time"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (ctl *CentralScheduler) findNodes() {
	// TODO add informer to get the list of nodes
	nodes, _ := ctl.clientset.CoreV1().Nodes().List(context.TODO(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s!=%s", "node-role.kubernetes.io/control-plane", ""),
	})
	nMap := make(map[string]int)

	for i, node := range nodes.Items {
		nMap[node.Name] = i
	}

	ctl.nodeMap = nMap
}


/* Binds Pod p to Node n */
func (ctl * CentralScheduler) placePodToNode(p *v1.Pod, n string) {
		ctl.clientset.CoreV1().Pods(p.Namespace).Bind(context.TODO(), &v1.Binding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      p.Name,
				Namespace: p.Namespace,
			},
			Target: v1.ObjectReference{
				APIVersion: "v1",
				Kind:       "Node",
				Name:       n,
			},
		}, metav1.CreateOptions{})
}

/* Creates Event detailing Binding */
func (ctl *CentralScheduler) createSchedEvent(p *v1.Pod, n string, eventTime time.Time,  annotations map[string]string) error {
    _, err := ctl.clientset.CoreV1().Events(p.Namespace).Create(context.TODO(), &v1.Event{
        Action:         "Binding",
        Message:        fmt.Sprintf("Successfully assigned %s/%s to %s", p.Namespace, p.Name, n),
        Reason:         "Scheduled",
        EventTime:      metav1.NewMicroTime(eventTime),
        Type:           "Normal",
        ReportingController: ctl.Name,
        ReportingInstance:   fmt.Sprintf("%s-%s", ctl.Name, os.Getenv("HOSTNAME")),
        InvolvedObject: v1.ObjectReference{
            Kind:      "Pod",
            Name:      p.Name,
            Namespace: p.Namespace,
            UID:       p.UID,
        },
        ObjectMeta: metav1.ObjectMeta{
            GenerateName: p.Name + "-",
            Annotations: annotations,
        },
    }, metav1.CreateOptions{})
	return err
}
