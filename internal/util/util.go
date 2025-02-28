package util

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute"
	"golang.org/x/crypto/ssh"

	"github.com/cloudbase/garm/params"
	"github.com/cloudbase/garm/runner/providers/common"
)

const (
	ControllerIDTagName = "garm-controller-id"
	PoolIDTagName       = "garm-pool-id"
)

var (
	powerStateMap = map[string]string{
		"PowerState/deallocated":  "stopped",
		"PowerState/deallocating": "stopped",
		"PowerState/running":      "running",
		"PowerState/starting":     "pending_create",
		"PowerState/stopped":      "stopped",
		"PowerState/stopping":     "stopped",
		"PowerState/unknown":      "unknown",
	}

	provisioningStateMap = map[string]string{
		"Creating":  "pending_create",
		"Updating":  "pending_create",
		"Migrating": "pending_create",
		"Failed":    "error",
		"Succeeded": "running",
		"Deleting":  "pending_delete",
	}
)

func TagsFromBootstrapParams(bootstrapParams params.BootstrapInstance, controllerID string) (map[string]*string, error) {
	ImageDetails, err := URNToImageDetails(bootstrapParams.Image)
	if err != nil {
		return nil, fmt.Errorf("failed to parse image: %w", err)
	}

	ret := map[string]*string{
		"os_arch":           to.Ptr(string(bootstrapParams.OSArch)),
		"os_version":        to.Ptr(ImageDetails.Version),
		"os_name":           to.Ptr(ImageDetails.SKU),
		"os_type":           to.Ptr(string(bootstrapParams.OSType)),
		PoolIDTagName:       to.Ptr(bootstrapParams.PoolID),
		ControllerIDTagName: to.Ptr(controllerID),
	}

	return ret, nil
}

type ImageDetails struct {
	Offer     string
	Publisher string
	SKU       string
	Version   string
}

func URNToImageDetails(urn string) (ImageDetails, error) {
	// MicrosoftWindowsServer:WindowsServer:2022-Datacenter:latest
	fields := strings.Split(urn, ":")
	if len(fields) != 4 {
		return ImageDetails{}, fmt.Errorf("invalid image URN: %s", urn)
	}

	return ImageDetails{
		Publisher: fields[0],
		Offer:     fields[1],
		SKU:       fields[2],
		Version:   fields[3],
	}, nil
}

func AzurePowerStateToGarmPowerState(vm armcompute.VirtualMachine) string {
	if vm.Properties != nil && vm.Properties.InstanceView != nil && vm.Properties.InstanceView.Statuses != nil {
		for _, val := range vm.Properties.InstanceView.Statuses {
			if val.Code != nil {
				code, ok := powerStateMap[*val.Code]
				if ok {
					return code
				}
			}
		}
	}

	if vm.Properties != nil && vm.Properties.ProvisioningState != nil {
		if status, ok := provisioningStateMap[*vm.Properties.ProvisioningState]; ok {
			return status
		}
	}

	return "unknown"
}

func AzureInstanceToParamsInstance(vm armcompute.VirtualMachine) (params.Instance, error) {
	if vm.Name == nil {
		return params.Instance{}, fmt.Errorf("missing VM name")
	}
	os_type, ok := vm.Tags["os_type"]
	if !ok {
		return params.Instance{}, fmt.Errorf("missing os_type tag in VM")
	}
	os_arch, ok := vm.Tags["os_arch"]
	if !ok {
		return params.Instance{}, fmt.Errorf("missing os_arch tag in VM")
	}
	os_version, ok := vm.Tags["os_version"]
	if !ok {
		return params.Instance{}, fmt.Errorf("missing os_version tag in VM")
	}
	os_name, ok := vm.Tags["os_name"]
	if !ok {
		return params.Instance{}, fmt.Errorf("missing os_name tag in VM")
	}
	return params.Instance{
		ProviderID: *vm.Name,
		Name:       *vm.Name,
		OSType:     params.OSType(*os_type),
		OSArch:     params.OSArch(*os_arch),
		OSName:     *os_name,
		OSVersion:  *os_version,
		Status:     common.InstanceStatus(AzurePowerStateToGarmPowerState(vm)),
	}, nil
}

// GenerateFakeKey generates a SSH key pair, returns the public key, and
// discards the private key. This is useful for droplets that don't need a
// public key, since DO & Azure insists on requiring one.
func GenerateFakeKey() (string, error) {
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", err
	}
	sshKey, err := ssh.NewPublicKey(&rsaKey.PublicKey)
	if err != nil {
		return "", err
	}
	return string(ssh.MarshalAuthorizedKey(sshKey)), nil
}
