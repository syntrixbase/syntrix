// MongoDB Replica Set Initialization Script
// This runs on first container startup

// Wait for MongoDB to be ready
sleep(2000);

// Initialize replica set if not already initialized
try {
  const status = rs.status();
  print("Replica set already initialized:", status.set);
} catch (e) {
  print("Initializing replica set...");
  rs.initiate({
    _id: "rs0",
    members: [{ _id: 0, host: "localhost:27017" }],
  });
  print("Replica set initialized successfully");
}
