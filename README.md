# EKS Lambda Sample

This code repository aims to be a reference sample for implementing AWS Lambda function to interact
with Amazon Elastic Container Services for Kubernetes (EKS) using Kubernetes API. Amazon EKS runs
the Kubernetes management infrastructure for you across multiple AWS availability zones to eliminate
a single point of failure. Amazon EKS is certified Kubernetes conformant so you can use existing
tooling and plugins from partners and the Kubernetes community. Applications running on any standard
Kubernetes environment are fully compatible and can be easily migrated to Amazon EKS.

Having a AWS Lambda function that integrates with Amazon EKS would allow event-driven triggers to
manage and modify deployments and other controls within the Amazon EKS cluster.

While this is just a sample that list the pods running within the Amazon EKS cluster, you can
modify the code to meet your needs.

## Prequisites

This sample requires the `go` compiler and `dep` to build. Refer to this [Getting Started
Guide](https://golang.org/doc/install) and [installing
dep](https://golang.github.io/dep/docs/installation.html) for detailed instructions.

## List of Samples

* [listpods](./lambda/listpods) - Most basic example of using this utility to list pods running in
  an Amazon EKS cluster.
* [codepipeline](./lambda/codepipeline) - Demonstration of Continuous Deployment using AWS
  CodePipeline to deploy an application to an Amazon EKS cluster.

## Configuring RBAC

You would also need to give the Lambda execution role permissions in Amazon EKS cluster. Refer to
this [User Guide](https://docs.aws.amazon.com/eks/latest/userguide/add-user-role.html) for detailed
instructions.

1. Edit the `aws-auth` ConfigMap of your cluster.
```
$ kubectl -n kube-system edit configmap/aws-auth
```
2. Add your Lambda execution role to the config
```
# Please edit the object below. Lines beginning with a '#' will be ignored,
# and an empty file will abort the edit. If an error occurs while saving this file will be
# reopened with the relevant failures.
#
apiVersion: v1
data:
  mapRoles: |
    - rolearn: arn:aws:iam::555555555555:role/devel-worker-nodes-NodeInstanceRole-74RF4UBDUKL6
      username: system:node:{{EC2PrivateDNSName}}
      groups:
        - system:bootstrappers
        - system:nodes
    - rolearn: arn:aws:iam::<AWS Account ID>:role/<your lambda execution role>
      username: admin
      groups:
        - system:masters
```

For your Lambda execution role, you will need permissions to describe EKS cluster. Add the following
statement to the IAM role.

```
{
    "Effect": "Allow",
    "Action": [
        "eks:DescribeCluster"
    ],
    "Resource": "*"
}
```

You may want to be more restrictive by specifying only the arn of your EKS cluster for resource
field.

Once these are configured, you can test your function. Good luck!



