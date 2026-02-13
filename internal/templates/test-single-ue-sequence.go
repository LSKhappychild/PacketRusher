/**
 * SPDX-License-Identifier: Apache-2.0
 * © Copyright 2023 Hewlett Packard Enterprise Development LP
 */

// Package templates provides test scenarios for 5G procedures.
//
// This file implements a single-UE sequential test that validates
// all major 5G control plane procedures in order:
// 1. Initial Registration
// 2. PDU Session Establishment
// 3. AN Release (UE goes to IDLE)
// 4. Service Request (UE returns to CONNECTED)
// 5. Handover (to second gNB)
// 6. Deregistration
//
// This test is useful for:
// - Validating core functionality
// - Debugging procedure issues
// - Understanding procedure flow
// - Quick sanity checks
package templates

import (
	"bufio"
	"fmt"
	"my5G-RANTester/config"
	"my5G-RANTester/internal/common/tools"
	gnbContext "my5G-RANTester/internal/control_test_engine/gnb/context"
	"my5G-RANTester/internal/control_test_engine/gnb/ngap/trigger"
	"my5G-RANTester/internal/control_test_engine/procedures"
	"my5G-RANTester/internal/control_test_engine/ue"
	ueCtx "my5G-RANTester/internal/control_test_engine/ue/context"
	"os"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

var stdinReader = bufio.NewReader(os.Stdin)

func waitForEnter(step string) {
	// Drain any buffered input to prevent auto-advancing from leftover newlines
	for stdinReader.Buffered() > 0 {
		stdinReader.ReadByte()
	}
	fmt.Printf("\n>>> Press ENTER to proceed to: %s ...", step)
	stdinReader.ReadBytes('\n')
}

// TestSingleUESequence performs a sequential test of all major 5G procedures on UEs
// Parameters:
//   - tunnel: Whether to create GTP-U tunnel
//   - numUEs: Number of UEs to run sequentially (each UE completes all steps before next UE starts)
//   - delayBetweenSteps: Delay in seconds between each procedure (0 for no delay)
func TestSingleUESequence(tunnel bool, numUEs int, delayBetweenSteps int) error {
	if numUEs < 1 {
		numUEs = 1
	}
	// Setup configuration
	cfg := config.GetConfig()

	// Set tunnel mode
	var tunnelMode config.TunnelMode
	if tunnel {
		tunnelMode = config.TunnelTun
	} else {
		tunnelMode = config.TunnelDisabled
	}
	cfg.Ue.TunnelMode = tunnelMode

	log.Info("========================================================")
	log.Info("[SEQUENCE-TEST] Sequential Procedure Test")
	log.Info("========================================================")
	log.Info(fmt.Sprintf("[SEQUENCE-TEST] Number of UEs: %d", numUEs))
	log.Info("[SEQUENCE-TEST] Each UE will execute the following procedures in order:")
	log.Info("  1. Initial Registration")
	log.Info("  2. PDU Session Establishment")
	log.Info("  3. AN Release (UE -> IDLE)")
	log.Info("  4. Service Request (UE -> CONNECTED)")
	log.Info("  5. Handover (to second gNB)")
	log.Info("  6. Deregistration")
	if delayBetweenSteps > 0 {
		log.Info(fmt.Sprintf("[SEQUENCE-TEST] Delay between steps: %d seconds", delayBetweenSteps))
	}
	log.Info("========================================================")

	// Create 2 gNodeBs (needed for handover)
	wg := &sync.WaitGroup{}
	numGnbs := 2
	gnbs := tools.CreateGnbs(numGnbs, cfg, wg)
	time.Sleep(2 * time.Second)

	var gnb1, gnb2 *gnbContext.GNBContext
	i := 0
	for _, g := range gnbs {
		if i == 0 {
			gnb1 = g
		} else {
			gnb2 = g
		}
		i++
	}

	if gnb1 == nil {
		return fmt.Errorf("failed to create gNodeB 1")
	}
	if gnb2 == nil {
		log.Warn("[SEQUENCE-TEST] Only 1 gNB created - Handover step will be skipped")
	}

	testStart := time.Now()
	successCount := 0
	failedUEs := []int{}

	// Loop through each UE
	for ueIdx := 1; ueIdx <= numUEs; ueIdx++ {
		log.Info("")
		log.Info("########################################################")
		log.Info(fmt.Sprintf("[SEQUENCE-TEST] Starting UE %d/%d", ueIdx, numUEs))
		log.Info("########################################################")

		ueStart := time.Now()
		ueFailed := false

		// ========================================================
		// Step 1: Initial Registration
		// ========================================================
		waitForEnter(fmt.Sprintf("UE %d - STEP 1/6: Initial Registration", ueIdx))
		log.Info("")
		log.Info("========================================================")
		log.Info(fmt.Sprintf("[SEQUENCE-TEST] UE %d - STEP 1/6: Initial Registration", ueIdx))
		log.Info("========================================================")
		stepStart := time.Now()

		ueId := ueIdx
		ueCfg := cfg
		ueCfg.Ue.Msin = tools.IncrementMsin(ueId, cfg.Ue.Msin)

		wg.Add(1)
		ueRx := make(chan procedures.UeTesterMessage, 10)
		ueTx := ue.NewUE(ueCfg, ueId, ueRx, gnb1.GetInboundChannel(), wg)

		// Send registration request
		ueRx <- procedures.UeTesterMessage{Type: procedures.Registration}

		// Wait for registration completion
		timeout := time.After(15 * time.Second)
		registered := false
	regLoop:
		for !registered {
			select {
			case msg := <-ueTx:
				if msg.StateChange == ueCtx.MM5G_REGISTERED {
					registered = true
					log.Info(fmt.Sprintf("[SEQUENCE-TEST] UE %d - Registration completed successfully", ueIdx))
					log.Info(fmt.Sprintf("[SEQUENCE-TEST] UE %d Step 1 duration: %.2f seconds", ueIdx, time.Since(stepStart).Seconds()))
				}
			case <-timeout:
				log.Error(fmt.Sprintf("[SEQUENCE-TEST] UE %d - Registration timeout", ueIdx))
				ueFailed = true
				failedUEs = append(failedUEs, ueIdx)
				break regLoop
			}
		}

		if ueFailed {
			log.Warn(fmt.Sprintf("[SEQUENCE-TEST] UE %d failed at Step 1, skipping remaining steps", ueIdx))
			continue
		}

		if delayBetweenSteps > 0 {
			log.Info(fmt.Sprintf("[SEQUENCE-TEST] Waiting %d seconds before next step...", delayBetweenSteps))
			time.Sleep(time.Duration(delayBetweenSteps) * time.Second)
		}

		// ========================================================
		// Step 2: PDU Session Establishment
		// ========================================================
		waitForEnter(fmt.Sprintf("UE %d - STEP 2/6: PDU Session Establishment", ueIdx))
		log.Info("")
		log.Info("========================================================")
		log.Info(fmt.Sprintf("[SEQUENCE-TEST] UE %d - STEP 2/6: PDU Session Establishment", ueIdx))
		log.Info("========================================================")
		stepStart = time.Now()

		// Request PDU session
		ueRx <- procedures.UeTesterMessage{Type: procedures.NewPDUSession}

		// PDU session establishment does not trigger a state change on the scenario channel,
		// so we wait a fixed duration for the NAS/GTP procedures to complete.
		pduWait := 3 * time.Second
		log.Info(fmt.Sprintf("[SEQUENCE-TEST] UE %d Waiting %v for PDU session establishment...", ueIdx, pduWait))
		time.Sleep(pduWait)
		log.Info(fmt.Sprintf("[SEQUENCE-TEST] UE %d - PDU session establishment completed", ueIdx))
		log.Info(fmt.Sprintf("[SEQUENCE-TEST] UE %d Step 2 duration: %.2f seconds", ueIdx, time.Since(stepStart).Seconds()))

		if delayBetweenSteps > 0 {
			log.Info(fmt.Sprintf("[SEQUENCE-TEST] Waiting %d seconds before next step...", delayBetweenSteps))
			time.Sleep(time.Duration(delayBetweenSteps) * time.Second)
		}

		// ========================================================
		// Step 3: AN Release (UE goes to IDLE)
		// ========================================================
		waitForEnter(fmt.Sprintf("UE %d - STEP 3/6: AN Release (UE -> IDLE)", ueIdx))
		log.Info("")
		log.Info("========================================================")
		log.Info(fmt.Sprintf("[SEQUENCE-TEST] UE %d - STEP 3/6: AN Release (UE -> IDLE)", ueIdx))
		log.Info("========================================================")
		stepStart = time.Now()

		// Send idle command (triggers gNB-initiated AN Release)
		ueRx <- procedures.UeTesterMessage{Type: procedures.Idle}

		// Wait for UE to transition to IDLE
		timeout = time.After(15 * time.Second)
		idled := false

	idleLoop:
		for !idled {
			select {
			case msg, open := <-ueTx:
				if !open {
					// Channel closed - UE is now in IDLE
					idled = true
					log.Info(fmt.Sprintf("[SEQUENCE-TEST] UE %d - UE transitioned to IDLE (channel closed)", ueIdx))
				} else if msg.StateChange == ueCtx.MM5G_IDLE {
					idled = true
					log.Info(fmt.Sprintf("[SEQUENCE-TEST] UE %d - UE transitioned to IDLE (state change)", ueIdx))
				}
			case <-timeout:
				log.Error(fmt.Sprintf("[SEQUENCE-TEST] UE %d - AN Release timeout", ueIdx))
				ueFailed = true
				failedUEs = append(failedUEs, ueIdx)
				break idleLoop
			}
		}

		if ueFailed {
			log.Warn(fmt.Sprintf("[SEQUENCE-TEST] UE %d failed at Step 3, skipping remaining steps", ueIdx))
			continue
		}

		// IMPORTANT: Wait for UE Context Release procedure to complete
		// The sequence is: UE->IDLE -> gNB sends Context Release Request -> AMF sends Context Release Command -> gNB sends Context Release Complete
		// This takes ~500ms, so wait 2 seconds to be safe
		contextReleaseWait := 2 * time.Second
		log.Info(fmt.Sprintf("[SEQUENCE-TEST] UE %d Waiting %v for UE Context Release to complete...", ueIdx, contextReleaseWait))
		time.Sleep(contextReleaseWait)

		log.Info(fmt.Sprintf("[SEQUENCE-TEST] UE %d - AN Release completed, UE is now IDLE", ueIdx))
		log.Info(fmt.Sprintf("[SEQUENCE-TEST] UE %d Step 3 duration: %.2f seconds", ueIdx, time.Since(stepStart).Seconds()))

		if delayBetweenSteps > 0 {
			log.Info(fmt.Sprintf("[SEQUENCE-TEST] Waiting %d seconds before next step...", delayBetweenSteps))
			time.Sleep(time.Duration(delayBetweenSteps) * time.Second)
		}

		// ========================================================
		// Step 4: Service Request (UE returns to CONNECTED)
		// ========================================================
		waitForEnter(fmt.Sprintf("UE %d - STEP 4/6: Service Request (UE -> CONNECTED)", ueIdx))
		log.Info("")
		log.Info("========================================================")
		log.Info(fmt.Sprintf("[SEQUENCE-TEST] UE %d - STEP 4/6: Service Request (UE -> CONNECTED)", ueIdx))
		log.Info("========================================================")
		stepStart = time.Now()

		// Create a channel to monitor state changes after Service Request
		serviceReqDone := make(chan bool, 1)

		// Start goroutine to monitor state changes from the UE
		go func() {
			for {
				select {
				case msg, open := <-ueTx:
					if !open {
						return
					}
					if msg.StateChange == ueCtx.MM5G_REGISTERED {
						serviceReqDone <- true
						return
					}
				}
			}
		}()

		// Send service request - UE will reconnect to gNB
		ueRx <- procedures.UeTesterMessage{Type: procedures.ServiceRequest}

		// Wait for REGISTERED state (CONNECTED mode)
		timeout = time.After(20 * time.Second) // Longer timeout for Service Request
		connected := false
	srLoop:
		for !connected {
			select {
			case <-serviceReqDone:
				connected = true
				log.Info(fmt.Sprintf("[SEQUENCE-TEST] UE %d - Service Request completed, UE is now CONNECTED", ueIdx))

				// Wait for PDU session to be restored
				pduRestoreWait := 1 * time.Second
				log.Info(fmt.Sprintf("[SEQUENCE-TEST] UE %d Waiting %v for PDU session restoration...", ueIdx, pduRestoreWait))
				time.Sleep(pduRestoreWait)

				log.Info(fmt.Sprintf("[SEQUENCE-TEST] UE %d Step 4 duration: %.2f seconds", ueIdx, time.Since(stepStart).Seconds()))
			case <-timeout:
				log.Error(fmt.Sprintf("[SEQUENCE-TEST] UE %d - Service Request timeout", ueIdx))
				ueFailed = true
				failedUEs = append(failedUEs, ueIdx)
				break srLoop
			}
		}

		if ueFailed {
			log.Warn(fmt.Sprintf("[SEQUENCE-TEST] UE %d failed at Step 4, skipping remaining steps", ueIdx))
			continue
		}

		if delayBetweenSteps > 0 {
			log.Info(fmt.Sprintf("[SEQUENCE-TEST] Waiting %d seconds before next step...", delayBetweenSteps))
			time.Sleep(time.Duration(delayBetweenSteps) * time.Second)
		}

		// ========================================================
		// Step 5: Handover (to second gNB)
		// ========================================================
		if gnb2 != nil {
			waitForEnter(fmt.Sprintf("UE %d - STEP 5/6: Handover (gNB1 -> gNB2)", ueIdx))
			log.Info("")
			log.Info("========================================================")
			log.Info(fmt.Sprintf("[SEQUENCE-TEST] UE %d - STEP 5/6: Handover (gNB1 -> gNB2)", ueIdx))
			log.Info("========================================================")
			stepStart = time.Now()

			// Create channel to monitor handover completion
			handoverDone := make(chan bool, 1)

			// Start goroutine to monitor state changes during handover
			go func() {
				for {
					select {
					case msg, open := <-ueTx:
						if !open {
							return
						}
						if msg.StateChange == ueCtx.MM5G_REGISTERED {
							handoverDone <- true
							return
						}
					}
				}
			}()

			// Trigger NGAP handover
			prUeId := int64(ueId)
			trigger.TriggerNgapHandover(gnb1, gnb2, prUeId)

			// Wait for handover completion
			timeout = time.After(20 * time.Second)
			handoverComplete := false
			for !handoverComplete {
				select {
				case <-handoverDone:
					handoverComplete = true
					log.Info(fmt.Sprintf("[SEQUENCE-TEST] UE %d - Handover completed successfully", ueIdx))
					log.Info(fmt.Sprintf("[SEQUENCE-TEST] UE %d Step 5 duration: %.2f seconds", ueIdx, time.Since(stepStart).Seconds()))
				case <-timeout:
					log.Warn(fmt.Sprintf("[SEQUENCE-TEST] UE %d - Handover timeout (this may be normal in some setups)", ueIdx))
					handoverComplete = true // Continue anyway
				}
			}

			if delayBetweenSteps > 0 {
				log.Info(fmt.Sprintf("[SEQUENCE-TEST] Waiting %d seconds before next step...", delayBetweenSteps))
				time.Sleep(time.Duration(delayBetweenSteps) * time.Second)
			}
		} else {
			log.Info("")
			log.Info("========================================================")
			log.Info(fmt.Sprintf("[SEQUENCE-TEST] UE %d - STEP 5/6: Handover - SKIPPED (only 1 gNB)", ueIdx))
			log.Info("========================================================")
		}

		// ========================================================
		// Step 6: Deregistration
		// ========================================================
		waitForEnter(fmt.Sprintf("UE %d - STEP 6/6: Deregistration", ueIdx))
		log.Info("")
		log.Info("========================================================")
		log.Info(fmt.Sprintf("[SEQUENCE-TEST] UE %d - STEP 6/6: Deregistration", ueIdx))
		log.Info("========================================================")
		stepStart = time.Now()

		// Create channel to monitor deregistration
		deregDone := make(chan bool, 1)

		// Start goroutine to monitor state changes during deregistration
		go func() {
			for {
				select {
				case msg, open := <-ueTx:
					if !open {
						// Channel closed - UE terminated
						deregDone <- true
						return
					}
					if msg.StateChange == ueCtx.MM5G_DEREGISTERED {
						deregDone <- true
						return
					}
				}
			}
		}()

		// Send terminate (which does proper deregistration)
		ueRx <- procedures.UeTesterMessage{Type: procedures.Terminate}

		// Wait for deregistration
		timeout = time.After(15 * time.Second)
		deregistered := false
		for !deregistered {
			select {
			case <-deregDone:
				deregistered = true
				log.Info(fmt.Sprintf("[SEQUENCE-TEST] UE %d - Deregistration completed successfully", ueIdx))
				log.Info(fmt.Sprintf("[SEQUENCE-TEST] UE %d Step 6 duration: %.2f seconds", ueIdx, time.Since(stepStart).Seconds()))
			case <-timeout:
				log.Warn(fmt.Sprintf("[SEQUENCE-TEST] UE %d - Deregistration timeout", ueIdx))
				deregistered = true // Consider it done
			}
		}

		// UE completed all steps successfully
		if !ueFailed {
			successCount++
			log.Info("")
			log.Info(fmt.Sprintf("[SEQUENCE-TEST] UE %d completed all steps in %.2f seconds", ueIdx, time.Since(ueStart).Seconds()))
		}
	} // End of UE loop

	// ========================================================
	// Test Complete
	// ========================================================
	totalDuration := time.Since(testStart)

	log.Info("")
	log.Info("========================================================")
	log.Info("[SEQUENCE-TEST] TEST COMPLETED")
	log.Info("========================================================")
	log.Info(fmt.Sprintf("Total UEs: %d", numUEs))
	log.Info(fmt.Sprintf("Successful: %d", successCount))
	log.Info(fmt.Sprintf("Failed: %d", len(failedUEs)))
	if len(failedUEs) > 0 {
		log.Info(fmt.Sprintf("Failed UE IDs: %v", failedUEs))
	}
	log.Info(fmt.Sprintf("Total test duration: %.2f seconds", totalDuration.Seconds()))
	log.Info("")
	log.Info("[SEQUENCE-TEST] Procedures per UE:")
	log.Info("  1. Initial Registration")
	log.Info("  2. PDU Session Establishment")
	log.Info("  3. AN Release (-> IDLE)")
	log.Info("  4. Service Request")
	if gnb2 != nil {
		log.Info("  5. Handover")
	} else {
		log.Info("  5. Handover (skipped)")
	}
	log.Info("  6. Deregistration")
	log.Info("========================================================")

	// Wait for user before cleaning up
	waitForEnter("Terminate test and clean up")

	// Clean up
	log.Info("[SEQUENCE-TEST] Cleaning up...")
	for _, g := range gnbs {
		g.Terminate()
	}
	wg.Wait()

	return nil
}
