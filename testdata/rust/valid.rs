/// A simple point in 2D space.
struct Point {
    x: f64,
    y: f64,
}

/// Represents different shapes.
enum Shape {
    Circle(f64),
    Rectangle(f64, f64),
    Triangle(f64, f64, f64),
}

/// A trait for objects that can be drawn.
trait Drawable {
    fn draw(&self);
    fn area(&self) -> f64;
}

impl Drawable for Shape {
    fn draw(&self) {
        println!("Drawing shape");
    }

    fn area(&self) -> f64 {
        0.0
    }
}

impl Point {
    /// Creates a new point.
    fn new(x: f64, y: f64) -> Self {
        Point { x, y }
    }

    fn distance(&self, other: &Point) -> f64 {
        ((self.x - other.x).powi(2) + (self.y - other.y).powi(2)).sqrt()
    }
}

/// A type alias for a list of points.
type PointList = Vec<Point>;

/// The maximum number of points allowed.
const MAX_POINTS: usize = 1000;

static ORIGIN: Point = Point { x: 0.0, y: 0.0 };

/// A helper macro for creating points.
macro_rules! point {
    ($x:expr, $y:expr) => {
        Point::new($x, $y)
    };
}

mod geometry {
    pub fn hello() {
        println!("Hello from geometry");
    }
}

fn main() {
    let p = point!(1.0, 2.0);
    println!("Point: ({}, {})", p.x, p.y);
}
