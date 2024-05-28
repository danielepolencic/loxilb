/*
 * Copyright (c) 2024 NetLOX Inc
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at:
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package loxinet

import (
	tk "github.com/loxilb-io/loxilib"
	"net"
)

func CloudUpdatePrivateIP(vIP net.IP, add bool) error {
	if mh.cloudLabel == "aws" {
		err := AWSUpdatePrivateIP(vIP, false)
		if err != nil {
			tk.LogIt(tk.LogError, "aws lb-rule vip %s delete failed. err: %v\n", vIP.String(), err)
		}
	} else if mh.cloudLabel == "ncloud" {
		err := nClient.NcloudUpdatePrivateIp(vIP, false)
		if err != nil {
			tk.LogIt(tk.LogError, "ncloud lb-rule vip %s delete failed. err: %v\n", vIP.String(), err)
		}
	}
	return nil
}