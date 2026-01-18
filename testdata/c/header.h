/**
 * @file header.h
 * @brief Sample header file for testing.
 */

#ifndef HEADER_H
#define HEADER_H

/**
 * @brief Maximum buffer size.
 */
#define MAX_BUFFER_SIZE 1024

/**
 * @brief Status codes for operations.
 */
typedef enum {
    STATUS_OK = 0,
    STATUS_ERROR = 1,
    STATUS_NOT_FOUND = 2
} Status;

/**
 * @brief Buffer structure for data storage.
 */
typedef struct {
    char data[MAX_BUFFER_SIZE];
    int length;
} Buffer;

/**
 * @brief Initialize a buffer.
 * @param buf Pointer to the buffer
 */
void buffer_init(Buffer* buf);

/**
 * @brief Write data to the buffer.
 * @param buf Pointer to the buffer
 * @param data Data to write
 * @param len Length of data
 * @return Status code
 */
Status buffer_write(Buffer* buf, const char* data, int len);

/**
 * @brief Read data from the buffer.
 * @param buf Pointer to the buffer
 * @param out Output buffer
 * @param max_len Maximum length to read
 * @return Number of bytes read
 */
int buffer_read(Buffer* buf, char* out, int max_len);

#endif /* HEADER_H */
