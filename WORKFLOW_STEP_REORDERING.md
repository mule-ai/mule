# Workflow Step Reordering Feature

This document explains the implementation of the drag-and-drop workflow step reordering feature in Mule AI.

## Overview

The workflow step reordering feature allows users to rearrange the order of steps within a workflow using intuitive drag-and-drop functionality in the web interface. This enables users to easily modify workflow execution sequences without having to recreate steps.

## Backend Implementation

### New API Endpoint

A new POST endpoint was added to handle step reordering:

```
POST /api/v1/workflows/{id}/steps/reorder
```

This endpoint accepts a JSON payload with an array of step IDs in their desired order:

```json
{
  "step_ids": ["step-3", "step-1", "step-2"]
}
```

### Database Changes

The `ReorderWorkflowSteps` method was added to the `WorkflowManager` in `/internal/manager/workflow.go`. This method:

1. Begins a database transaction for atomicity
2. Verifies that all provided step IDs belong to the specified workflow
3. Updates the `step_order` field for each workflow step according to the new sequence
4. Commits the transaction to persist changes

The implementation ensures data integrity by validating that all step IDs belong to the workflow before making any changes.

### Handler Implementation

In `/cmd/api/handlers.go`, the `reorderWorkflowStepsHandler` method:

1. Validates that the workflow exists
2. Parses the request body to extract the ordered list of step IDs
3. Calls the workflow manager's `ReorderWorkflowSteps` method
4. Returns the updated list of workflow steps

## Frontend Implementation

### Drag-and-Drop Library

The feature utilizes the `@dnd-kit` library for drag-and-drop functionality:

- `@dnd-kit/core` for core drag-and-drop functionality
- `@dnd-kit/sortable` for sortable list components

### Components

#### SortableWorkflowSteps.js

A new component `/frontend/src/components/SortableWorkflowSteps.js` was created to represent individual workflow steps with drag handles:

- Each step displays a grab handle icon for drag initiation
- Visual feedback during dragging (opacity change)
- Displays step information including type, associated agent or WASM module
- Shows current step order

#### WorkflowBuilder.js Modifications

Significant changes were made to `/frontend/src/pages/WorkflowBuilder.js`:

1. **Drag Context Setup**: 
   - Configured sensors for pointer and keyboard interactions
   - Implemented collision detection and sorting strategies

2. **Drag Event Handling**:
   - Added `handleDragEnd` function to process reordering
   - Uses `arrayMove` utility to reorder steps in the UI immediately
   - Sends updated order to backend API via `workflowsAPI.reorderSteps`
   - Implements error handling and rollback on failure

3. **UI Integration**:
   - Wrapped workflow steps list with `DndContext` and `SortableContext`
   - Replaced static list items with `SortableWorkflowStepItem` components
   - Maintains visual feedback during drag operations

### API Service

The `/frontend/src/services/api.js` file was updated with a new method:

```javascript
reorderSteps: (id, stepIds) => api.post(`/api/v1/workflows/${id}/steps/reorder`, { step_ids: stepIds })
```

## How to Use the Feature

1. Navigate to the Workflow Builder page in the web interface
2. Select an existing workflow from the list
3. View the workflow steps in the right panel
4. Click and hold the drag handle (three horizontal lines) on any step
5. Drag the step to a new position in the list
6. Release the mouse button to drop the step in its new position
7. The system automatically saves the new order and updates all step numbers

The reordering happens in real-time with immediate visual feedback. If the backend operation fails, the UI will revert to the previous state and display an error message in the browser console.

## Technical Details

### Data Flow

1. User initiates drag operation in the frontend
2. Frontend immediately reorders items in the UI for responsiveness
3. New order is sent to the backend API endpoint
4. Backend validates and persists the new order in the database
5. Updated step list is returned from the backend
6. Frontend refreshes the display with confirmed order

### Error Handling

- Frontend rolls back UI changes if backend operation fails
- Backend transactions ensure database consistency
- Validation prevents invalid step IDs from being reordered

### Performance Considerations

- Database operations use transactions to ensure atomicity
- Minimal data transfer (only step IDs, not full step objects)
- Immediate UI updates provide responsive user experience