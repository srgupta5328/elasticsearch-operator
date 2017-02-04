/*
Copyright (c) 2017, UPMC Enterprises
All rights reserved.
Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met:
    * Redistributions of source code must retain the above copyright
      notice, this list of conditions and the following disclaimer.
    * Redistributions in binary form must reproduce the above copyright
      notice, this list of conditions and the following disclaimer in the
      documentation and/or other materials provided with the distribution.
    * Neither the name UPMC Enterprises nor the
      names of its contributors may be used to endorse or promote products
      derived from this software without specific prior written permission.
THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND
ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
DISCLAIMED. IN NO EVENT SHALL UPMC ENTERPRISES BE LIABLE FOR ANY
DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES
(INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES;
LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND
ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
(INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THIS
*/

package processor

import (
	"sync"

	"github.com/Sirupsen/logrus"
	"github.com/upmc-enterprises/elasticsearch-operator/util/k8sutil"
)

// processorLock ensures that reconciliation and event processing does
// not happen at the same time.
var processorLock = &sync.Mutex{}

// Processor object
type Processor struct {
	k8sclient *k8sutil.K8sutil
	baseImage string
}

// New creates new instance of Processor
func New(kclient *k8sutil.K8sutil, baseImage string) (*Processor, error) {
	p := &Processor{
		k8sclient: kclient,
		baseImage: baseImage,
	}

	return p, nil
}

// WatchElasticSearchClusterEvents watches for changes to tpr elasticsearch events
func (p *Processor) WatchElasticSearchClusterEvents(done chan struct{}, wg *sync.WaitGroup) {
	events, watchErrs := p.k8sclient.MonitorElasticSearchEvents()
	go func() {
		for {
			select {
			case event := <-events:
				err := p.processElasticSearchClusterEvent(event)
				if err != nil {
					logrus.Errorln(err)
				}
			case err := <-watchErrs:
				logrus.Errorln(err)
			case <-done:
				wg.Done()
				logrus.Println("Stopped elasticsearch event watcher.")
				return
			}
		}
	}()
}

func (p *Processor) processElasticSearchClusterEvent(c k8sutil.ElasticSearchEvent) error {
	processorLock.Lock()
	defer processorLock.Unlock()
	switch {
	case c.Type == "ADDED":
	case c.Type == "MODIFIED":
		return p.processElasticSearchCluster(c.Object)
	case c.Type == "DELETED":
		return p.deleteElasticSearchCluster(c.Object)
	}
	return nil
}

func (p *Processor) processElasticSearchCluster(c k8sutil.ElasticSearchCluster) error {
	logrus.Println("--------> ElasticSearch Event!")

	// Is a base image defined in the custom cluster?
	var baseImage = p.calcBaseImage(p.baseImage, c.Spec.ElasticSearchImage)

	// Create Services
	p.k8sclient.CreateDiscoveryService()
	p.k8sclient.CreateDataService()
	p.k8sclient.CreateClientService()

	// Create Storage Classes
	for _, sc := range c.Spec.Zones {
		p.k8sclient.CreateStorageClass(sc)
	}

	p.k8sclient.CreateClientMasterDeployment("client", baseImage, &c.Spec.ClientNodeReplicas)
	p.k8sclient.CreateClientMasterDeployment("master", baseImage, &c.Spec.MasterNodeReplicas)

	zoneDistribution := p.calculateZoneDistribution(c.Spec.DataNodeReplicas, len(c.Spec.Zones))

	for index, count := range zoneDistribution {
		p.k8sclient.CreateDataNodeDeployment(&count, p.baseImage, c.Spec.Zones[index], c.Spec.DataDiskSize)
	}

	return nil
}

func (p *Processor) deleteElasticSearchCluster(c k8sutil.ElasticSearchCluster) error {
	logrus.Println("--------> ElasticSearch Cluster deleted...removing all components...")

	err := p.k8sclient.DeleteClientMasterDeployment("client")
	if err != nil {
		logrus.Error("Could not delete client deployment:", err)
	}

	err = p.k8sclient.DeleteClientMasterDeployment("master")
	if err != nil {
		logrus.Error("Could not delete master deployment:", err)
	}

	err = p.k8sclient.DeleteStatefulSet()
	if err != nil {
		logrus.Error("Could not delete stateful set:", err)
	}

	p.k8sclient.DeleteServices()

	// Leave PV + PVC's for now?
	return nil
}

func (p *Processor) calculateZoneDistribution(dataReplicas, zonesCount int) []int32 {
	if zonesCount == 0 {
		zonesCount = 1
	}

	zoneDistribution := make([]int32, zonesCount)

	index := 0
	for i := 0; i < dataReplicas; i++ {
		if index >= zonesCount {
			index = 0
		}

		zoneDistribution[index]++
		index++
	}

	return zoneDistribution
}

func (p *Processor) calcBaseImage(baseImage, customImage string) string {
	if len(customImage) > 0 {
		baseImage = customImage
	}

	return baseImage
}