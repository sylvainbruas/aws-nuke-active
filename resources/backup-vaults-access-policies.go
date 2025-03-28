package resources

import (
	"context"

	"github.com/ekristen/aws-nuke/v3/pkg/nuke"
	"github.com/ekristen/libnuke/pkg/registry"
	"github.com/ekristen/libnuke/pkg/resource"

	"fmt"

	"github.com/aws/aws-sdk-go/service/backup"
)

const AWSBackupVaultAccessPolicyResource = "AWSBackupVaultAccessPolicy"

func init() {
	registry.Register(&registry.Registration{
		Name:     AWSBackupVaultAccessPolicyResource,
		Scope:    nuke.Account,
		Resource: &BackupVaultAccessPolicy{},
		Lister:   &AWSBackupVaultAccessPolicyLister{},
	})
}

type AWSBackupVaultAccessPolicyLister struct{}

func (l *AWSBackupVaultAccessPolicyLister) List(_ context.Context, o interface{}) ([]resource.Resource, error) {
	opts := o.(*nuke.ListerOpts)

	svc := backup.New(opts.Session)
	maxVaultsLen := int64(100)
	params := &backup.ListBackupVaultsInput{
		MaxResults: &maxVaultsLen, // aws default limit on number of backup vaults per account
	}
	resp, err := svc.ListBackupVaults(params)
	if err != nil {
		return nil, err
	}

	// Iterate over backup vaults and add vault policies that exist.
	resources := make([]resource.Resource, 0)
	for _, out := range resp.BackupVaultList {
		// Check if the Backup Vault has an Access Policy set
		resp, err := svc.GetBackupVaultAccessPolicy(&backup.GetBackupVaultAccessPolicyInput{
			BackupVaultName: out.BackupVaultName,
		})

		// Non-existent Access Policies can come from ResourceNotFoundException or
		// being nil.
		if err != nil {
			switch err.(type) {
			case *backup.ResourceNotFoundException:
				// Non-existent is OK and we skip over them
				continue
			default:
				return nil, err
			}
		}

		// Only delete policies that exist.
		if resp.Policy != nil {
			resources = append(resources, &BackupVaultAccessPolicy{
				svc:             svc,
				accountID:       opts.AccountID,
				backupVaultName: *out.BackupVaultName,
			})
		}
	}

	return resources, nil
}

type BackupVaultAccessPolicy struct {
	svc             *backup.Backup
	accountID       *string
	backupVaultName string
}

func (b *BackupVaultAccessPolicy) Remove(_ context.Context) error {
	// Set the policy to a policy that allows deletion before removal.
	//
	// This is required to delete the policy for the automagically created vaults
	// such as "aws/efs/automatic-backup-vault" from EFS automatic backups
	// which by default Deny policy deletion via backup::DeleteBackupVaultAccessPolicy
	//
	// Example "aws/efs/automatic-backup-vault" default policy:
	//
	// {
	//     "Version": "2012-10-17",
	//     "Statement": [
	//         {
	//             "Effect": "Deny",
	//             "Principal": {
	//                 "AWS": "*"
	//             },
	//             "Action": [
	//                 "backup:DeleteBackupVault",
	//                 "backup:DeleteBackupVaultAccessPolicy",
	//                 "backup:DeleteRecoveryPoint",
	//                 "backup:StartCopyJob",
	//                 "backup:StartRestoreJob",
	//                 "backup:UpdateRecoveryPointLifecycle"
	//             ],
	//             "Resource": "*"
	//         }
	//     ]
	// }
	//
	// While deletion is Denied, you can update the policy with one that
	// doesn't deny and then delete at will.
	allowDeletionPolicy := fmt.Sprintf(`{
	"Version": "2012-10-17",
	"Statement": [
		{
			"Effect": "Allow",
			"Principal": {
				"AWS": "arn:aws:iam::%s:root"
			},
			"Action": "backup:DeleteBackupVaultAccessPolicy",
			"Resource": "*"
		}
	]
}`, *b.accountID)
	// Ignore error from if we can't put permissive backup vault policy in for some reason, that's OK.
	_, _ = b.svc.PutBackupVaultAccessPolicy(&backup.PutBackupVaultAccessPolicyInput{
		BackupVaultName: &b.backupVaultName,
		Policy:          &allowDeletionPolicy,
	})
	// In the end, this is the only call we actually really care about for err.
	_, err := b.svc.DeleteBackupVaultAccessPolicy(&backup.DeleteBackupVaultAccessPolicyInput{
		BackupVaultName: &b.backupVaultName,
	})
	return err
}

func (b *BackupVaultAccessPolicy) String() string {
	return b.backupVaultName
}
