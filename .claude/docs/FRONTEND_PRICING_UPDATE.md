# Frontend Pricing Page Update - MVP Launch

## Overview
The backend API has been updated for the MVP launch. Only 2 plans are active (Trial + Solo), with 2 more marked as "Coming Soon".

## API Changes

### Endpoint
`GET /api/v1/subscription-plans` (no authentication required)

### New Fields Available
```typescript
interface SubscriptionPlan {
  // Existing fields
  id: string;
  name: string;
  description: string;
  price_amount: number; // in cents (€9.00 = 900)
  currency: string;
  billing_interval: string;
  features: string[];
  is_active: boolean; // ⭐ USE THIS TO SHOW/HIDE PLANS

  // NEW Terminal-specific fields
  max_session_duration_minutes: number;
  max_concurrent_terminals: number;
  allowed_machine_sizes: string[]; // ["XS"], ["S"], ["M"], ["L"]
  network_access_enabled: boolean;
  data_persistence_enabled: boolean;
  data_persistence_gb: number;
  allowed_templates: string[];

  // NEW: Planned features (announced but not yet available)
  planned_features: string[]; // Features coming soon (🔜 prefix)
}
```

**Note on Planned Features:**
- Use the `planned_features` array to show upcoming features for each plan
- These are features that will be added in the future but aren't available yet
- Display them with a "Coming Soon" badge or grayed out style
- Example: `["🔜 200MB persistent storage", "🔜 Web development with port forwarding"]`

## Required Changes

### 1. Filter Plans by Active Status
```typescript
// Fetch all plans
const response = await fetch('http://localhost:8080/api/v1/subscription-plans');
const { data: allPlans } = await response.json();

// Split active and coming soon
const activePlans = allPlans.filter(plan => plan.is_active);
const comingSoonPlans = allPlans.filter(plan => !plan.is_active);
```

### 2. Update Pricing Cards

**For Active Plans (Trial & Solo):**
- Show normal pricing card
- Enable "Subscribe" / "Get Started" button
- Display all features from `features` array
- Show machine size from `allowed_machine_sizes[0]`

**For Coming Soon Plans (Trainer & Organization):**
- Gray out or add opacity overlay
- Add "Coming Soon" badge
- **Disable** all purchase/subscribe buttons
- Keep pricing visible but mark as unavailable
- Optional: Add "Notify me" button for interest

### 3. Display Current and Planned Features

```typescript
// Example display logic
const displayPlan = (plan) => {
  const price = plan.price_amount / 100; // Convert cents to euros
  const machineSize = plan.allowed_machine_sizes[0]; // "XS", "S", "M", "L"
  const sessionHours = plan.max_session_duration_minutes / 60;
  const storage = plan.data_persistence_enabled
    ? `${plan.data_persistence_gb}GB`
    : 'Ephemeral only';

  return {
    title: plan.name,
    price: `€${price}`,
    isAvailable: plan.is_active,
    currentFeatures: [
      ...plan.features, // Use existing features array
      `${sessionHours}h max session`,
      `${plan.max_concurrent_terminals} concurrent terminal${plan.max_concurrent_terminals > 1 ? 's' : ''}`,
      `${machineSize} machine size`,
      storage,
    ],
    // NEW: Show planned features with special styling
    plannedFeatures: plan.planned_features || [], // Array of upcoming features
  };
};
```

**Displaying Planned Features:**
```jsx
{/* Current features - normal style */}
{plan.currentFeatures.map(feature => (
  <li key={feature}>{feature}</li>
))}

{/* Planned features - grayed out or with badge */}
{plan.plannedFeatures.length > 0 && (
  <div className="planned-features">
    <h4>Coming Soon</h4>
    {plan.plannedFeatures.map(feature => (
      <li key={feature} className="text-gray-400">
        {feature} {/* Already includes 🔜 emoji */}
      </li>
    ))}
  </div>
)}
```

### 4. Update Button Behavior

```jsx
<button
  disabled={!plan.is_active}
  onClick={() => plan.is_active && handleSubscribe(plan.id)}
  className={plan.is_active ? 'btn-primary' : 'btn-disabled'}
>
  {plan.is_active ? 'Subscribe Now' : 'Coming Soon'}
</button>
```

## Current Active Plans

### ✅ Trial (FREE)
- **Status**: ACTIVE - Ready to purchase
- **Machine**: XS (0.5 vCPU, 256MB RAM)
- **Session**: 1 hour max
- **Concurrent**: 1 terminal
- **Network**: ❌ No network access
- **Storage**: Ephemeral only

### ✅ Solo (€9/month)
- **Status**: ACTIVE - Ready to purchase
- **Machine**: S (1 vCPU, 1GB RAM)
- **Session**: 8 hours max
- **Concurrent**: 1 terminal
- **Network**: Outbound access
- **Storage**: Ephemeral only
- **Planned Features**:
  - 🔜 200MB persistent storage

### ❌ Trainer (€19/month) - COMING SOON
- **Status**: INACTIVE - Do not allow purchase
- **Machine**: M (2 vCPU, 2GB RAM)
- **Session**: 8 hours max
- **Concurrent**: 3 terminals
- **Network**: Outbound access
- **Storage**: Ephemeral only
- **Planned Features**:
  - 🔜 1GB persistent storage
  - 🔜 Web development with port forwarding
  - 🔜 Custom images
  - 🔜 Team collaboration features

### ❌ Organization (€49/month) - COMING SOON
- **Status**: INACTIVE - Do not allow purchase
- **Machine**: L (4 vCPU, 4GB RAM)
- **Session**: 8 hours max
- **Concurrent**: 10 terminals
- **Network**: Outbound access
- **Storage**: Ephemeral only
- **Planned Features**:
  - 🔜 5GB persistent storage
  - 🔜 Web development with port forwarding
  - 🔜 Custom images
  - 🔜 Team collaboration features
  - 🔜 Priority support

## Design Recommendations

1. **Active Plans**: Full color, normal opacity, clickable
2. **Coming Soon Plans**:
   - Grayscale or 50% opacity
   - "Coming Soon" badge in top-right corner
   - Price visible but grayed out
   - Disabled button with "Notify Me" option
3. **Machine Size Labels**: Display prominently (XS, S, M, L)
4. **Session Duration**: Show as "Xh sessions" for clarity

## Terminal Sessions - Machine Size Information

**Important Update**: Terminal sessions now include the **actual machine size used** from Terminal Trainer.

### Terminal Session Response
```typescript
interface TerminalSession {
  id: string;
  session_id: string;
  user_id: string;
  status: string; // "active", "stopped", "expired"
  expires_at: string;
  instance_type: string; // "ubuntu", "python", etc.
  machine_size: string; // ⭐ NEW: Actual size used (XS, S, M, L, XL)
  created_at: string;
}
```

### How It Works

1. **Subscription Plan** defines `allowed_machine_sizes: ["XS"]` - what sizes the user CAN use
2. **Terminal Trainer** decides the actual size and returns it when creating a session
3. **Terminal Session** stores and exposes `machine_size: "XS"` - what size is ACTUALLY being used

### Example API Call
```bash
# Get user's active terminals
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/terminal-sessions/user-sessions
```

**Response:**
```json
{
  "data": [
    {
      "id": "...",
      "session_id": "abc123",
      "status": "active",
      "instance_type": "ubuntu",
      "machine_size": "XS",  // ⭐ Actual size used
      "expires_at": "2025-10-08T14:00:00Z"
    }
  ]
}
```

### Display Recommendations

Show the **actual machine size** in the terminal list:
- "Ubuntu terminal (XS - 0.5 vCPU, 256MB)"
- Use the subscription plan's `allowed_machine_sizes` to show what sizes are available
- Use the terminal session's `machine_size` to show what size is currently running

## Testing

Test the subscription plans endpoint:
```bash
curl http://localhost:8080/api/v1/subscription-plans | jq '.data[] | {name, is_active, price_amount, allowed_machine_sizes}'
```

Expected output:
- Trial: `is_active: true, allowed_machine_sizes: ["XS"]`
- Solo: `is_active: true, allowed_machine_sizes: ["S"]`
- Trainer: `is_active: false, allowed_machine_sizes: ["M"]`
- Organization: `is_active: false, allowed_machine_sizes: ["L"]`

Test terminal sessions (requires authentication):
```bash
curl -H "Authorization: Bearer $TOKEN" \
  http://localhost:8080/api/v1/terminal-sessions/user-sessions
```

Expected: Each terminal includes `machine_size` field with actual size used.

## Questions?

Contact backend team or check API documentation at: http://localhost:8080/swagger/
