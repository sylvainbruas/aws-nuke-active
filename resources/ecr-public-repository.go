package resources

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/service/ecrpublic"

	"github.com/ekristen/libnuke/pkg/registry"
	"github.com/ekristen/libnuke/pkg/resource"
	"github.com/ekristen/libnuke/pkg/types"

	"github.com/ekristen/aws-nuke/v3/pkg/nuke"
)

const ECRPublicRepositoryResource = "ECRPublicRepository"

func init() {
	registry.Register(&registry.Registration{
		Name:     ECRPublicRepositoryResource,
		Scope:    nuke.Account,
		Resource: &ECRPublicRepository{},
		Lister:   &ECRPublicRepositoryLister{},
		DependsOn: []string{
			EC2VPNGatewayAttachmentResource,
		},
		AlternativeResource: "AWS::ECR::PublicRepository",
	})
}

type ECRPublicRepositoryLister struct{}

func (l *ECRPublicRepositoryLister) List(_ context.Context, o interface{}) ([]resource.Resource, error) {
	opts := o.(*nuke.ListerOpts)
	svc := ecrpublic.New(opts.Session)
	var resources []resource.Resource

	// ECRPublicRepository is only supported in us-east-1, only run if the region is us-east-1
	if opts.Session.Config.Region == nil || *opts.Session.Config.Region != endpoints.UsEast1RegionID {
		return resources, nil
	}

	input := &ecrpublic.DescribeRepositoriesInput{
		MaxResults: aws.Int64(100),
	}

	for {
		output, err := svc.DescribeRepositories(input)
		if err != nil {
			return nil, err
		}

		for _, repository := range output.Repositories {
			tagResp, err := svc.ListTagsForResource(&ecrpublic.ListTagsForResourceInput{
				ResourceArn: repository.RepositoryArn,
			})
			if err != nil {
				return nil, err
			}
			resources = append(resources, &ECRPublicRepository{
				svc:         svc,
				name:        repository.RepositoryName,
				createdTime: repository.CreatedAt,
				tags:        tagResp.Tags,
			})
		}

		if output.NextToken == nil {
			break
		}

		input.NextToken = output.NextToken
	}

	return resources, nil
}

type ECRPublicRepository struct {
	svc         *ecrpublic.ECRPublic
	name        *string
	createdTime *time.Time
	tags        []*ecrpublic.Tag
}

func (r *ECRPublicRepository) Filter() error {
	return nil
}

func (r *ECRPublicRepository) Properties() types.Properties {
	props := types.NewProperties()
	props.Set("CreatedTime", r.createdTime.Format(time.RFC3339))

	for _, t := range r.tags {
		props.SetTag(t.Key, t.Value)
	}

	return props
}

func (r *ECRPublicRepository) Remove(_ context.Context) error {
	params := &ecrpublic.DeleteRepositoryInput{
		RepositoryName: r.name,
		Force:          aws.Bool(true),
	}
	_, err := r.svc.DeleteRepository(params)
	return err
}

func (r *ECRPublicRepository) String() string {
	return *r.name
}
