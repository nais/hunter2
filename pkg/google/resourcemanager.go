package google

import (
	"context"
	"fmt"
	"google.golang.org/api/cloudresourcemanager/v1"
	"strings"
)

type ResourceManagerClient interface {
	GetProjectName(ctx context.Context, projectID string) (string, error)
}

type resourceManagerClient struct {
	*cloudresourcemanager.Service
}

func (r *resourceManagerClient) GetProjectName(ctx context.Context, projectID string) (string, error) {
	project, err := r.Projects.Get(projectID).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("fetching project: %v", err)
	}
	return ExtractProjectName(project.Name), nil
}

func NewResourceManagerClient(ctx context.Context) (ResourceManagerClient, error) {
	service, err := cloudresourcemanager.NewService(ctx)
	if err != nil {
		return nil, fmt.Errorf("creating resource manager service: %w", err)
	}
	return &resourceManagerClient{service}, nil
}

func ExtractProjectName(projectName string) string {
	lastIndexOfDash := strings.LastIndex(projectName, "-")

	if lastIndexOfDash > -1 {
		return projectName[:lastIndexOfDash]
	}

	return projectName
}
