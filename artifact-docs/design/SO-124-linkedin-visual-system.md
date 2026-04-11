# Design Spec: LinkedIn Visual Support (SO-124)

This document defines the lightweight visual system for the "Agent-Orchestrated Development" LinkedIn post series. The goal is to improve comprehension of complex orchestration concepts without requiring high-fidelity illustration or net-new brand exploration.

## 1. Visual Strategy: "The Operator's Schematic"

The visual system leans into the **operator-led, technical, and concrete** tone of SecondOrder. Instead of marketing illustrations, we use "schematics" that look like simplified abstractions of the product UI itself.

### Key Principles
- **Density over Fluff:** Use diagrams that actually explain the logic (flows, stacks, boards).
- **UI-First:** Visual elements should feel like they were pulled from a high-quality technical dashboard.
- **Brand Continuity:** Use the warm cream (`#f9f8f3`) for light mode posts and zinc-950 (`#09090b`) for dark mode posts.

## 2. Visual vs. Text-Only Guidance

| Post Theme | Format | Rationale |
|---|---|---|
| **Post 1: Team Operations** | **Carousel (4 slides)** | The "A vs B" contrast of single-agent vs. team-operations is too dense for a single image. |
| **Post 2: Coordination** | **Text-Only** | This post is highly rhetorical and punchy. A diagram might actually distract from the "Who owns this?" questions. |
| **Post 3: Security** | **Static Diagram** | Security flows (keys, scopes, runtimes) are best understood as a linear pipeline. |
| **Post 4: Recovery** | **Carousel (3 slides)** | Visualizing the *time* aspect of recovery (Failure -> Detection -> Restart) requires sequential slides. |
| **Post 5: Persistent State** | **Static Comparison** | A side-by-side comparison of "Chat Context" vs. "System State" is a powerful "aha" moment. |
| **Post 6: Provider Diversity** | **Static Diagram** | An ecosystem map showing different models feeding one control plane is a classic, high-comprehension visual. |

## 3. The Visual System (Design Tokens)

### Backgrounds
- **Primary:** `#f9f8f3` (Warm Cream) - default for most posts to feel "open" and readable.
- **Secondary:** `#09090b` (Zinc-950) - use for "Security" or "Deep Tech" posts to signal technical depth.

### Typography (Inter)
- **Headlines:** 60px, Bold, Tracking -0.02em.
- **Body/Labels:** 32px, Medium.
- **Monospace (Code/IDs):** 28px, Regular (JetBrains Mono or system fallback).

### Accent Elements
- **Lines:** 4px stroke width. Use `#009f60` (Green) for flows and "Success" paths.
- **Containers:** 2px border `#dfdedd` (Light) or `#27272a` (Dark). 8px border-radius.
- **The "Signature":** Every visual must have the 3px top accent line (`#009f60` or `#4f46e5`) and the "2ND Order" wordmark in the bottom right.

## 4. Concrete Asset Concepts

### Concept 1: The "Unit vs. System" Carousel (Post 1)
- **Slide 1:** Title: "A Single Agent is a Work Unit." Visual: A single chat bubble with a generic "AI" icon.
- **Slide 2:** Title: "A Team Needs Operations." Visual: A simplified 2x2 grid representing a Board, Wiki, Logs, and Reviewers.
- **Slide 3:** Title: "The Operating Layer." Visual: Connect the "Work Unit" (chat bubble) as a small component *inside* the grid from Slide 2.
- **Slide 4:** CTA: "Orchestration > Prompting."

### Concept 2: The "Security Pipeline" (Post 3)
- **Layout:** Horizontal flow (Left to Right).
- **Step 1:** "Trigger" (Icon: Lightning bolt).
- **Step 2:** "Provision" (Icon: Key, Label: "Fresh Per-Run API Key").
- **Step 3:** "Execution" (Icon: Terminal, Label: "Scoped Assignee Permissions").
- **Step 4:** "Boundary" (Icon: Shield, Label: "Runtime Heart Protection").
- **Visual Style:** Use the accent green for the arrows connecting the steps.

### Concept 3: The "Memory Stack" (Post 5)
- **Left Side:** Label: "Context Window (Transient)". Visual: A tall, semi-transparent bucket filled with "Chat History". Arrow pointing to "Garbage Collection" at the bottom.
- **Right Side:** Label: "System State (Durable)". Visual: A solid stack of horizontal blocks labeled "Issues", "Wiki", "Run Logs", "Costs".
- **Middle:** A "Vs" divider.
- **Insight:** "Context for the task. System for the organization."

## 5. Layout & Framing Guidance

- **Aspect Ratio:** 1:1 (1080x1080px) for all assets to ensure compatibility with both LinkedIn mobile and desktop feeds.
- **Safe Zones:** Keep all text and core diagram elements within a 90px inner margin.
- **Accessibility:** 
    - Ensure contrast ratios for text on background are at least 4.5:1. 
    - Use the Accent Green (`#009f60`) sparingly as a highlight, not as a text color for long passages.
- **Export Settings:** Export as PNG-24 for sharpest rendering of text and lines.
