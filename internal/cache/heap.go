package cache

// MinHeap struct has a slice that holds the array
type MinHeap struct {
	slice []int
}

// Inserts adds an element to the heap
func (h *MinHeap) Insert(key int) {
	h.slice = append(h.slice, key)
	h.minHeapifyUp(len(h.slice) - 1)
}

// Extract returns the smallest key, and removes it from the heap.
func (h *MinHeap) Extract() (int, bool) {
	// cannot extract from an empty heap
	if len(h.slice) == 0 {
		return 0, false
	}

	extracted := h.slice[0]
	n := len(h.slice) - 1

	// when there is only one element
	if n == 0 {
		h.slice = h.slice[:0]
		return extracted, true
	}

	// take out the last index
	h.slice[0] = h.slice[n]

	h.slice = h.slice[:n]

	h.minHeapifyDown(0)

	return extracted, true
}

// minHeapifyUp will heapify from bottom to top
func (h *MinHeap) minHeapifyUp(index int) {
	for index > 0 {
		p := parent(index)

		if h.slice[p] <= h.slice[index] {
			return
		}

		h.swap(p, index)
		index = p
	}
}

// minHeapifyDown will heapify from top to bottom
func (h *MinHeap) minHeapifyDown(index int) {
	n := len(h.slice)

	// loop while index has atleast one child
	for {
		l, r := left(index), right(index)

		// if there is no left child, there are no children
		if l >= n {
			return
		}

		childToCompare := l

		// when the right child exists, compare both children
		if r < n && h.slice[r] < h.slice[l] {
			childToCompare = r
		}

		// compare slice value of current index to smaller child
		// and swap if current value is larger
		if h.slice[index] <= h.slice[childToCompare] {
			return
		}

		h.swap(index, childToCompare)
		index = childToCompare
	}
}

// get the parent
func parent(i int) int {
	return (i - 1) / 2
}

// get the left child
func left(i int) int {
	return 2*i + 1
}

// get the right child
func right(i int) int {
	return 2*i + 2
}

// swap keys in the array
func (h *MinHeap) swap(i1, i2 int) {
	h.slice[i1], h.slice[i2] = h.slice[i2], h.slice[i1]
}
