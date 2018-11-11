package dpbuilder

import (
	"errors"
	"fmt"

	artifacts "github.com/kun-lun/artifacts/pkg/apis"
	deployments "github.com/kun-lun/artifacts/pkg/apis/deployments"
)

type deploymentItem struct {
	hostGroup  deployments.HostGroup
	deployment deployments.Deployment
}

type DeploymentBuilder struct{}

func (dp DeploymentBuilder) Produce(
	manifest artifacts.Manifest,
) ([]deployments.HostGroup, []deployments.Deployment, error) {
	// generate the deployments
	deploymentItems := []deploymentItem{}
	for _, item := range manifest.VMGroups {
		if item.Roles != nil && len(item.Roles) > 0 {
			hostGroup, err := dp.produceHostGroup(item)
			if err != nil {
				return nil, nil, err
			}
			deployment, err := dp.generateDeployment(item)
			if err != nil {
				return nil, nil, err
			}
			deploymentItems = append(deploymentItems, deploymentItem{
				hostGroup:  hostGroup,
				deployment: deployment,
			})
		}
	}
	// generate the ansible scripts based on the deployments.
	hostGroups := []deployments.HostGroup{}
	deployments := []deployments.Deployment{}
	for _, di := range deploymentItems {
		hostGroups = append(hostGroups, di.hostGroup)
		deployments = append(deployments, di.deployment)
	}

	dp.provisionJumpboxParameters(hostGroups)
	return hostGroups, deployments, nil
}

func (dp DeploymentBuilder) provisionJumpboxParameters(hostGroups []deployments.HostGroup) {
	// find the ip of the jumpbox
	var (
		jumpboxUser string
		jumpboxHost string
	)
	for _, hostGroup := range hostGroups {
		if hostGroup.GroupType == deployments.JumpboxHostGroupType {
			jumpboxUser = hostGroup.Hosts[0].User
			jumpboxHost = hostGroup.Hosts[0].Host
		}
	}
	for _, hostGroup := range hostGroups {
		if hostGroup.GroupType != deployments.JumpboxHostGroupType {
			for index := range hostGroup.Hosts {
				hostGroup.Hosts[index].SSHCommonArgs = fmt.Sprintf("-o ProxyCommand=\"ssh -W %%h:%%p -q %s@%s\"", jumpboxUser, jumpboxHost)
			}
		}
	}
}

func (dp DeploymentBuilder) produceHostGroup(vmGroup artifacts.VMGroup) (deployments.HostGroup, error) {
	hostGroup := deployments.HostGroup{}
	hostGroup.Name = dp.generateHostGroupName(vmGroup)

	networkInfos := vmGroup.NetworkInfos

	if len(networkInfos) == 0 || len(networkInfos[0].Outputs) == 0 {
		return deployments.HostGroup{},
			fmt.Errorf("no network info or outputs found in group %s, %d, %d", vmGroup.Name, len(networkInfos), len(networkInfos[0].Outputs))
	}
	host := deployments.Host{}
	host.User = vmGroup.OSProfile.AdminName
	if dp.isJumpbox(vmGroup) {
		hostGroup.GroupType = deployments.JumpboxHostGroupType
		if vmGroup.Count != 1 {
			return deployments.HostGroup{}, errors.New("jumpbox count should be one")
		}
		host.Alias = hostGroup.Name
		if networkInfos[0].Outputs[0].Host == "" {
			host.Host = networkInfos[0].Outputs[0].PublicIP
		} else {
			host.Host = networkInfos[0].Outputs[0].Host
		}
		hostGroup.Hosts = append(hostGroup.Hosts, host)
	} else {
		if vmGroup.Count != len(networkInfos[0].Outputs) {
			return deployments.HostGroup{}, errors.New("the outputs number does not match the vm group")
		}
		firstNetworkInfoOutputs := networkInfos[0].Outputs
		for i := 0; i < vmGroup.Count; i++ {
			// TODO(andliu) think about a better way to generate the alias.
			host.Alias = firstNetworkInfoOutputs[i].IP
			host.Host = firstNetworkInfoOutputs[i].IP

			hostGroup.Hosts = append(hostGroup.Hosts, host)
		}
	}

	return hostGroup, nil
}

func (dp DeploymentBuilder) isJumpbox(vmGroup artifacts.VMGroup) bool {
	for _, item := range vmGroup.Meta {
		if item.Key == "group_type" && item.Value == deployments.JumpboxHostGroupType {
			return true
		}
	}
	return false
}

func (dp DeploymentBuilder) generateHostGroupName(vmGroup artifacts.VMGroup) string {
	return vmGroup.Name
}

func (dp DeploymentBuilder) generateDeployment(vmGroup artifacts.VMGroup) (deployments.Deployment, error) {
	deployment := deployments.Deployment{}
	deployment.HostGroupName = dp.generateHostGroupName(vmGroup)

	for _, role := range vmGroup.Roles {
		deployment.Roles = append(deployment.Roles, artifacts.Role{
			Name: role.Name,
		})
		// append the vars
		deployment.Vars = role.Vars
	}
	return deployment, nil
}
