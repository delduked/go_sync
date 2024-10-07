#!/bin/bash

# Define the output file
output_file="all_contents.txt"

# Remove the output file if it exists
if [ -f "$output_file" ]; then
    rm "$output_file"
fi

# Find all .go and .proto files, excluding those ending with pb.go
find . -type f \( -name "*.go" -o -name "*.proto" \) ! -name "*pb.go" | while read -r file; do
    # Append a comment with the file name to the output file
    echo "// Contents of $file" >> "$output_file"
    # Append the contents of the file to the output file
    cat "$file" >> "$output_file"
    # Add a newline for separation
    echo "" >> "$output_file"
done

echo "All contents have been written to $output_file"