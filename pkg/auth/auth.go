package auth

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/kubernetes-sigs/aws-iam-authenticator/pkg/token"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/eks"
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/aws/aws-sdk-go/service/sts/stsiface"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	clientset "k8s.io/client-go/kubernetes"
)

// NewAuthClient creates a new EKS authenticated clientset.
func NewAuthClient(config *ClusterConfig) (*clientset.Clientset, error) {
	// Start new AWS session if not specified
	if config.Session == nil {
		config.Session = newSession()
	}

	// Load the rest from AWS using SDK
	err := config.loadConfig()
	if err != nil {
		return nil, errors.Wrap(err, "Unable to load Kubernetes Client Config")
	}

	// Create the Kubernetes client
	client, err := config.NewClientConfig()
	if err != nil {
		return nil, errors.Wrap(err, "Unable to create Kubernetes Client Config")
	}

	clientset, err := client.NewClientSetWithEmbeddedToken()
	if err != nil {
		return nil, errors.Wrap(err, "Unable to create Kubernetes Client Set")
	}

	return clientset, nil
}

// Retrieve EKS cluster endpoint and CA from AWS
func (c *ClusterConfig) loadConfig() error {
	if c.ClusterName == "" {
		errors.New("ClusterName cannot be empty")
	}

	svc := eks.New(c.Session)
	input := &eks.DescribeClusterInput{
		Name: aws.String(c.ClusterName),
	}

	log.WithField("cluster", c.ClusterName).Info("Looking up EKS cluster")

	result, err := svc.DescribeCluster(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			log.WithField("cluster", c.ClusterName).Error(aerr.Error())
			return errors.Wrap(err, aerr.Error())
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			log.WithField("cluster", c.ClusterName).Error(err.Error())
			return errors.Wrap(err, err.Error())
		}
	}

	log.WithField("cluster", c.ClusterName).Info("Found cluster")
	log.WithField("cluster", result.Cluster).Debug("Cluster details")

	c.MasterEndpoint = *result.Cluster.Endpoint
	c.CertificateAuthorityData = *result.Cluster.CertificateAuthority.Data
	return nil
}

func (c *ClusterConfig) NewClientConfig() (*ClientConfig, error) {

	stsAPI := sts.New(c.Session)

	iamRoleARN, err := checkAuth(stsAPI)
	if err != nil {
		return nil, err
	}
	contextName := fmt.Sprintf("%s@%s", getUsername(iamRoleARN), c.ClusterName)

	data, err := base64.StdEncoding.DecodeString(c.CertificateAuthorityData)
	if err != nil {
		return nil, errors.Wrap(err, "decoding certificate authority data")
	}

	log.Info("Creating Kubernetes client config")
	clientConfig := &ClientConfig{
		Client: &clientcmdapi.Config{
			Clusters: map[string]*clientcmdapi.Cluster{
				c.ClusterName: {
					Server:                   c.MasterEndpoint,
					CertificateAuthorityData: data,
				},
			},
			Contexts: map[string]*clientcmdapi.Context{
				contextName: {
					Cluster:  c.ClusterName,
					AuthInfo: contextName,
				},
			},
			AuthInfos: map[string]*clientcmdapi.AuthInfo{
				contextName: &clientcmdapi.AuthInfo{},
			},
			CurrentContext: contextName,
		},
		ClusterName: c.ClusterName,
		ContextName: contextName,
		roleARN:     iamRoleARN,
		sts:         stsAPI,
	}

	return clientConfig, nil

}

func newSession() *session.Session {
	config := aws.NewConfig()
	config = config.WithCredentialsChainVerboseErrors(true)

	opts := session.Options{
		Config:                  *config,
		SharedConfigState:       session.SharedConfigEnable,
		AssumeRoleTokenProvider: stscreds.StdinTokenProvider,
	}

	stscreds.DefaultDuration = 30 * time.Minute

	return session.Must(session.NewSessionWithOptions(opts))
}

func checkAuth(stsAPI stsiface.STSAPI) (string, error) {
	input := &sts.GetCallerIdentityInput{}
	output, err := stsAPI.GetCallerIdentity(input)
	if err != nil {
		return "", errors.Wrap(err, "checking AWS STS access – cannot get role ARN for current session")
	}
	iamRoleARN := *output.Arn
	log.Debugf("role ARN for the current session is %s", iamRoleARN)
	return iamRoleARN, nil
}

type ClusterConfig struct {
	ClusterName              string
	MasterEndpoint           string
	CertificateAuthorityData string
	Session                  *session.Session
}

type ClientConfig struct {
	Client      *clientcmdapi.Config
	ClusterName string
	ContextName string
	roleARN     string
	sts         stsiface.STSAPI
}

func getUsername(iamRoleARN string) string {
	usernameParts := strings.Split(iamRoleARN, "/")
	if len(usernameParts) > 1 {
		return usernameParts[len(usernameParts)-1]
	}
	return "iam-root-account"
}

func (c *ClientConfig) WithEmbeddedToken() (*ClientConfig, error) {
	clientConfigCopy := *c

	log.Info("Generating token")

	gen, err := token.NewGenerator(true, false)
	if err != nil {
		return nil, errors.Wrap(err, "could not get token generator")
	}

	tok, err := gen.GetWithSTS(c.ClusterName, c.sts.(*sts.STS))
	if err != nil {
		return nil, errors.Wrap(err, "could not get token")
	}

	x := c.Client.AuthInfos[c.ContextName]
	x.Token = tok.Token

	log.WithField("token", tok).Debug("Successfully generated token")
	return &clientConfigCopy, nil
}

func (c *ClientConfig) NewClientSetWithEmbeddedToken() (*clientset.Clientset, error) {
	clientConfig, err := c.WithEmbeddedToken()
	if err != nil {
		return nil, errors.Wrap(err, "creating Kubernetes client config with embedded token")
	}
	clientSet, err := clientConfig.NewClientSet()
	if err != nil {
		return nil, errors.Wrap(err, "creating Kubernetes client")
	}
	return clientSet, nil
}

func (c *ClientConfig) NewClientSet() (*clientset.Clientset, error) {
	clientConfig, err := clientcmd.NewDefaultClientConfig(*c.Client, &clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		return nil, errors.Wrap(err, "failed to create API client configuration from client config")
	}

	client, err := clientset.NewForConfig(clientConfig)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create API client")
	}
	return client, nil
}
