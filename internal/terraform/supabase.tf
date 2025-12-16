variable "supabase_project_id" {
  description = "Supabase Project ID"
  type        = string
}

variable "supabase_service_key" {
  description = "Supabase Service Key"
  type        = string
  sensitive   = true
}

variable "webhook_secret" {
  description = "Secret for webhook validation"
  type        = string
  sensitive   = true
}

resource "supabase_project" "mule_events" {
  name = "mule-events"
  organization_id = var.supabase_project_id
}

resource "supabase_database_function" "webhook_handler" {
  project_id = supabase_project.mule_events.id
  name       = "handle_webhook"
  schema     = "public"
  
  # Simplified function for handling webhooks
  body = <<-EOT
    DECLARE
      payload JSONB;
    BEGIN
      payload := NEW.*;
      -- Store the webhook event in our events table
      INSERT INTO webhook_events (event_type, payload, created_at)
      VALUES (TG_ARGV[0], payload, NOW());
      RETURN NEW;
    END;
  EOT
  
  returns = "trigger"
}

resource "supabase_database_table" "webhook_events" {
  project_id = supabase_project.mule_events.id
  name       = "webhook_events"
  
  column {
    name = "id"
    type = "uuid"
    default_value = "gen_random_uuid()"
    is_primary_key = true
  }
  
  column {
    name = "event_type"
    type = "text"
  }
  
  column {
    name = "payload"
    type = "jsonb"
  }
  
  column {
    name = "created_at"
    type = "timestamp with time zone"
    default_value = "now()"
  }
  
  column {
    name = "processed"
    type = "boolean"
    default_value = false
  }
}