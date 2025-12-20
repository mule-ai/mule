class WebSocketService {
  constructor() {
    this.ws = null;
    this.listeners = {};
    this.reconnectInterval = 1000;
    this.maxReconnectInterval = 30000;
    this.reconnectDecay = 1.5;
    this.reconnectAttempts = 0;
    this.maxReconnectAttempts = 10;
    this.forcedClose = false;
    this.reconnectTimeoutId = null;
  }

  connect() {
    // Close any existing connection
    if (this.ws) {
      this.forcedClose = true;
      this.ws.close();
    }

    // Reset reconnection attempts when manually connecting
    this.forcedClose = false;
    this.reconnectAttempts = 0;

    // Determine WebSocket URL based on current protocol and host
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/ws`;

    console.log(`Attempting to connect to WebSocket at ${wsUrl}`);

    try {
      this.ws = new WebSocket(wsUrl);
      this.setupEventHandlers();
    } catch (error) {
      console.error('Failed to create WebSocket connection:', error);
      this.publish('connection_status', { connected: false });
      this.handleReconnect();
    }
  }

  setupEventHandlers() {
    this.ws.onopen = () => {
      console.log('WebSocket connected');
      this.reconnectAttempts = 0; // Reset reconnection attempts on successful connection
      this.publish('connection_status', { connected: true });
    };

    this.ws.onclose = (event) => {
      console.log('WebSocket closed', event);
      this.publish('connection_status', { connected: false });

      // Reconnect if not forced close and within max attempts
      if (!this.forcedClose && this.reconnectAttempts < this.maxReconnectAttempts) {
        this.handleReconnect();
      }
    };

    this.ws.onerror = (error) => {
      console.error('WebSocket error:', error);
      // Publish connection status as false on error
      this.publish('connection_status', { connected: false });
      this.publish('connection_error', { error });
    };

    this.ws.onmessage = (event) => {
      try {
        const message = JSON.parse(event.data);
        this.handleMessage(message);
      } catch (error) {
        console.error('Error parsing WebSocket message:', error);
      }
    };
  }

  handleMessage(message) {
    // Publish the message to all subscribers of its type
    this.publish(message.type, message.data);
    
    // Also publish to general message subscribers
    this.publish('message', message);
  }

  handleReconnect() {
    // If we've exceeded max attempts, stop trying
    if (this.reconnectAttempts >= this.maxReconnectAttempts) {
      console.log('Max reconnection attempts reached, stopping reconnection attempts');
      return;
    }

    // If this is the first reconnect attempt, do it quickly
    // Otherwise, use exponential backoff
    let timeout;
    if (this.reconnectAttempts === 0) {
      timeout = this.reconnectInterval;
    } else {
      timeout = Math.min(
        this.reconnectInterval * Math.pow(this.reconnectDecay, this.reconnectAttempts),
        this.maxReconnectInterval
      );
    }

    this.reconnectAttempts++;

    console.log(`Attempting to reconnect in ${timeout}ms (attempt ${this.reconnectAttempts})`);

    this.reconnectTimeoutId = setTimeout(() => {
      this.connect();
    }, timeout);
  }

  subscribe(eventType, callback) {
    if (!this.listeners[eventType]) {
      this.listeners[eventType] = [];
    }
    
    this.listeners[eventType].push(callback);
  }

  unsubscribe(eventType, callback) {
    if (!this.listeners[eventType]) return;
    
    const index = this.listeners[eventType].indexOf(callback);
    if (index > -1) {
      this.listeners[eventType].splice(index, 1);
    }
  }

  publish(eventType, data) {
    if (!this.listeners[eventType]) return;
    
    // Call all listeners for this event type
    this.listeners[eventType].forEach(callback => {
      try {
        callback(data);
      } catch (error) {
        console.error('Error in WebSocket listener:', error);
      }
    });
  }

  disconnect() {
    this.forcedClose = true;
    
    if (this.reconnectTimeoutId) {
      clearTimeout(this.reconnectTimeoutId);
    }
    
    if (this.ws) {
      this.ws.close();
    }
  }

  isConnected() {
    return this.ws && this.ws.readyState === WebSocket.OPEN;
  }
}

// Export singleton instance
const webSocketService = new WebSocketService();
export default webSocketService;