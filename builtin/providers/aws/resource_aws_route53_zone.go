package aws

import (
	"log"
	"strings"
	"time"

	"github.com/hashicorp/terraform/helper/resource"
	"github.com/hashicorp/terraform/helper/schema"

	"github.com/awslabs/aws-sdk-go/aws"
	"github.com/awslabs/aws-sdk-go/gen/route53"
)

func resourceAwsRoute53Zone() *schema.Resource {
	return &schema.Resource{
		Create: resourceAwsRoute53ZoneCreate,
		Read:   resourceAwsRoute53ZoneRead,
		Delete: resourceAwsRoute53ZoneDelete,

		Schema: map[string]*schema.Schema{
			"name": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"zone_id": &schema.Schema{
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func resourceAwsRoute53ZoneCreate(d *schema.ResourceData, meta interface{}) error {
	r53 := meta.(*AWSClient).r53conn

	comment := &route53.HostedZoneConfig{Comment: aws.String("Managed by Terraform")}
	req := &route53.CreateHostedZoneRequest{
		Name:             aws.String(d.Get("name").(string)),
		HostedZoneConfig: comment,
		CallerReference:  aws.String(time.Now().Format(time.RFC3339Nano)),
	}
	log.Printf("[DEBUG] Creating Route53 hosted zone: %s", req.Name)
	resp, err := r53.CreateHostedZone(req)
	if err != nil {
		return err
	}

	// Store the zone_id
	zone := CleanZoneID(*resp.HostedZone.ID)
	d.Set("zone_id", zone)
	d.SetId(zone)

	// Wait until we are done initializing
	wait := resource.StateChangeConf{
		Delay:      30 * time.Second,
		Pending:    []string{"PENDING"},
		Target:     "INSYNC",
		Timeout:    10 * time.Minute,
		MinTimeout: 2 * time.Second,
		Refresh: func() (result interface{}, state string, err error) {
			changeRequest := &route53.GetChangeRequest{
				ID: aws.String(CleanChangeID(*resp.ChangeInfo.ID)),
			}
			return resourceAwsGoRoute53Wait(r53, changeRequest)
		},
	}
	_, err = wait.WaitForState()
	if err != nil {
		return err
	}
	return nil
}

func resourceAwsRoute53ZoneRead(d *schema.ResourceData, meta interface{}) error {
	r53 := meta.(*AWSClient).r53conn
	_, err := r53.GetHostedZone(&route53.GetHostedZoneRequest{ID: aws.String(d.Id())})
	if err != nil {
		// Handle a deleted zone
		if strings.Contains(err.Error(), "404") {
			d.SetId("")
			return nil
		}
		return err
	}

	return nil
}

func resourceAwsRoute53ZoneDelete(d *schema.ResourceData, meta interface{}) error {
	r53 := meta.(*AWSClient).r53conn

	log.Printf("[DEBUG] Deleting Route53 hosted zone: %s (ID: %s)",
		d.Get("name").(string), d.Id())
	_, err := r53.DeleteHostedZone(&route53.DeleteHostedZoneRequest{ID: aws.String(d.Id())})
	if err != nil {
		return err
	}

	return nil
}

func resourceAwsGoRoute53Wait(r53 *route53.Route53, ref *route53.GetChangeRequest) (result interface{}, state string, err error) {

	status, err := r53.GetChange(ref)
	if err != nil {
		return nil, "UNKNOWN", err
	}
	return true, *status.ChangeInfo.Status, nil
}

// CleanChangeID is used to remove the leading /change/
func CleanChangeID(ID string) string {
	if strings.HasPrefix(ID, "/change/") {
		ID = strings.TrimPrefix(ID, "/change/")
	}
	return ID
}
