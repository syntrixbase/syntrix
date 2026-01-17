package evaluator

// All tests from this file have been consolidated into publisher_coverage_test.go:
// - TestNatsPublisher_Publish -> TestTaskPublisher_Publish/success
// - TestNatsPublisher_Publish_HashedSubject -> TestTaskPublisher_Publish/long subject gets hashed
// - TestNatsPublisher_Publish_Error -> TestTaskPublisher_Publish/publish error
// - TestNatsPublisher_Close_Delegates -> TestTaskPublisher_Close
// - TestNewTaskPublisher_EmptyStreamName -> TestNewTaskPublisher/with empty stream name uses default
