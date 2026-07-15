import React from 'react';
import { useSortable } from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { ListGroup, Button } from 'react-bootstrap';

// Component for a sortable workflow step item
function SortableWorkflowStepItem({ step, index, agents, wasmModules, onEdit, onDelete }) {
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: step.id });

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
    opacity: isDragging ? 0.5 : 1,
    cursor: 'grab',
  };

  return (
    <ListGroup.Item
      ref={setNodeRef}
      style={style}
      {...attributes}
      className="d-flex justify-content-between align-items-center"
    >
      <div className="d-flex align-items-center">
        <div 
          {...listeners}
          className="me-2"
          style={{ cursor: 'grab' }}
        >
          <svg width="16" height="16" viewBox="0 0 16 16" className="text-muted">
            <path fill="currentColor" d="M7 2a2 2 0 1 1-4 0 2 2 0 0 1 4 0zm0 5a2 2 0 1 1-4 0 2 2 0 0 1 4 0zm0 5a2 2 0 1 1-4 0 2 2 0 0 1 4 0zM15 2a2 2 0 1 1-4 0 2 2 0 0 1 4 0zm0 5a2 2 0 1 1-4 0 2 2 0 0 1 4 0zm0 5a2 2 0 1 1-4 0 2 2 0 0 1 4 0z"/>
          </svg>
        </div>
        <div>
          <strong>Step {index + 1}:</strong> {step.type}
          {step.agent_id && (
            <span className="ms-2 badge bg-primary">
              {agents.find(a => a.id === step.agent_id)?.name || 'Unknown Agent'}
            </span>
          )}
          {step.wasm_module_id && (
            <span className="ms-2 badge bg-success">
              {wasmModules.find(m => m.id === step.wasm_module_id)?.name || 'Unknown WASM Module'}
            </span>
          )}
        </div>
      </div>
      <div>
        <small className="text-muted me-2">Order: {step.step_order}</small>
        <Button
          variant="outline-primary"
          size="sm"
          className="me-1"
          onClick={() => onEdit(step)}
        >
          Edit
        </Button>
        <Button
          variant="outline-danger"
          size="sm"
          onClick={() => onDelete(step.id)}
        >
          Delete
        </Button>
      </div>
    </ListGroup.Item>
  );
}

export default SortableWorkflowStepItem;