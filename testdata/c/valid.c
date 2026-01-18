/**
 * @file valid.c
 * @brief Sample C file for testing.
 */

#include <stdio.h>
#include <stdlib.h>

/**
 * @brief A simple point structure.
 */
struct Point {
    int x;
    int y;
};

/**
 * @brief Creates a new point.
 * @param x The x coordinate
 * @param y The y coordinate
 * @return A new Point structure
 */
struct Point create_point(int x, int y) {
    struct Point p;
    p.x = x;
    p.y = y;
    return p;
}

/**
 * @brief Calculates the distance from origin.
 * @param p The point
 * @return The squared distance from origin
 */
int distance_squared(struct Point p) {
    return p.x * p.x + p.y * p.y;
}

/**
 * @brief Prints a point to stdout.
 * @param p The point to print
 */
void print_point(struct Point p) {
    printf("Point(%d, %d)\n", p.x, p.y);
}

// Simple helper function
int add(int a, int b) {
    return a + b;
}

int main(void) {
    struct Point p = create_point(3, 4);
    print_point(p);
    printf("Distance squared: %d\n", distance_squared(p));
    return 0;
}
