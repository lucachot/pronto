#!/bin/bash

# Deploy A.yaml
echo "Deploying remote-sched.yaml..."
kubectl apply -f deploy/test-remote-sched.yaml

# Check if the deployment of A.yaml was successful
if [ $? -eq 0 ]; then
  echo "remote-sched.yaml deployed successfully. Waiting a few seconds before deploying pi-job.yaml..."
  sleep 45 # Wait for 5 seconds (you can adjust this value)

  echo "Deploying mem-stress.yaml..."
  kubectl apply -f deploy/cpu-stress.yaml
  sleep 45 # Wait for 5 seconds (you can adjust this value)

  # Deploy B.yaml
  echo "Deploying pi-job.yaml..."
  kubectl apply -f deploy/pi-job.yaml

  # Check if the deployment of pi-job.yaml was successful
  if [ $? -eq 0 ]; then
    echo "pi-job.yaml deployed successfully."
  else
    echo "Error: Failed to deploy pi-job.yaml."
    exit 1
  fi
else
  echo "Error: Failed to deploy remote-sched.yaml. Aborting deployment of pi-job.yaml."
  exit 1
fi

echo "Deployment process complete."
