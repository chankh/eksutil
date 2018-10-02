package main

import (
	"context"
	"encoding/json"
	"os"

	eksauth "github.com/chankh/eksutil/pkg/auth"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	log "github.com/sirupsen/logrus"
)

func main() {
	if os.Getenv("ENV") == "DEBUG" {
		log.SetLevel(log.DebugLevel)
	}

	lambda.Start(handler)
}

func handler(context context.Context, req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {

	// Setup the basic EKS cluster info
	cfg := &eksauth.ClusterConfig{
		ClusterName: os.Getenv("CLUSTER_NAME"),
	}

	clientset, err := eksauth.NewAuthClient(cfg)
	if err != nil {
		log.WithError(err).Fatal(err.Error())
	}

	// Call Kubernetes API here
	pods, err := clientset.CoreV1().Pods("").List(metav1.ListOptions{})
	if err != nil {
		log.WithError(err).Fatal("Error listing pods")
	}

	var results []string

	for i, pod := range pods.Items {
		log.Infof("[%d] %s", i, pod.Name)
		results = append(results, pod.Name)
	}

	json, err := json.Marshal(results)
	if err != nil {
		log.WithError(err).Fatal("Unable to marshal results to json")

	}

	return events.APIGatewayProxyResponse{Body: string(json), StatusCode: 200}, nil
}
